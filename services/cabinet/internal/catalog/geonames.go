package catalog

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// GeoNamesSource reads the official cities500 ZIP and countryInfo TSV formats.
// URLs and resource limits are supplied by the application configuration.
type GeoNamesSource struct {
	Client                                 *http.Client
	CitiesURL, CountriesURL                string
	MaxDownloadBytes, MaxUncompressedBytes int64
	MaxCities                              int
}

func (s GeoNamesSource) download(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.New("invalid catalog URL")
	}
	req.Header.Set("User-Agent", "TravelWatch-catalog-importer")
	res, err := s.Client.Do(req)
	if err != nil {
		return nil, errors.New("catalog download failed or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog HTTP status %d", res.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, s.MaxDownloadBytes+1))
	if err != nil {
		return nil, errors.New("cannot read catalog download")
	}
	if int64(len(b)) > s.MaxDownloadBytes {
		return nil, errors.New("catalog download exceeds limit")
	}
	return b, nil
}
func (s GeoNamesSource) Load(ctx context.Context) (Snapshot, error) {
	if s.Client == nil || s.MaxDownloadBytes <= 0 || s.MaxUncompressedBytes <= 0 || s.MaxCities <= 0 {
		return Snapshot{}, errors.New("invalid GeoNames source limits")
	}
	countries, err := s.download(ctx, s.CountriesURL)
	if err != nil {
		return Snapshot{}, err
	}
	cities, err := s.download(ctx, s.CitiesURL)
	if err != nil {
		return Snapshot{}, err
	}
	digest := sha256.New()
	digest.Write(countries)
	digest.Write(cities)
	result := Snapshot{Source: "geonames", Version: hex.EncodeToString(digest.Sum(nil))}
	err = lines(ctx, bytes.NewReader(countries), func(fields []string) error {
		if len(fields) < 19 {
			return errors.New("invalid countryInfo record")
		}
		result.Countries = append(result.Countries, Country{Code: fields[0], Name: fields[4]})
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	archive, err := zip.NewReader(bytes.NewReader(cities), int64(len(cities)))
	if err != nil {
		return Snapshot{}, errors.New("invalid cities ZIP")
	}
	if len(archive.File) != 1 || archive.File[0].FileInfo().IsDir() {
		return Snapshot{}, errors.New("expected one cities file in ZIP")
	}
	file := archive.File[0]
	if file.UncompressedSize64 > uint64(s.MaxUncompressedBytes) {
		return Snapshot{}, errors.New("uncompressed catalog exceeds limit")
	}
	reader, err := file.Open()
	if err != nil {
		return Snapshot{}, errors.New("cannot open cities ZIP entry")
	}
	defer reader.Close()
	limited := &io.LimitedReader{R: reader, N: s.MaxUncompressedBytes + 1}
	err = lines(ctx, limited, func(f []string) error {
		if len(result.Cities) >= s.MaxCities {
			return errors.New("city count exceeds limit")
		}
		if len(f) != 19 || f[6] != "P" {
			return errors.New("invalid GeoNames city record")
		}
		id, e1 := strconv.ParseInt(f[0], 10, 64)
		lat, e2 := strconv.ParseFloat(f[4], 64)
		lon, e3 := strconv.ParseFloat(f[5], 64)
		pop, e4 := strconv.ParseInt(f[14], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil {
			return errors.New("invalid GeoNames numeric field")
		}
		aliases := []string{}
		seen := map[string]bool{f[1]: true}
		for _, a := range append([]string{f[2]}, strings.Split(f[3], ",")...) {
			if a != "" && !seen[a] {
				aliases = append(aliases, a)
				seen[a] = true
			}
		}
		result.Cities = append(result.Cities, City{SourceID: id, Name: f[1], CountryCode: f[8], RegionCode: f[10], Timezone: f[17], Aliases: aliases, Latitude: lat, Longitude: lon, Population: pop})
		return nil
	})
	if err != nil {
		return Snapshot{}, err
	}
	if limited.N <= 0 {
		return Snapshot{}, errors.New("uncompressed catalog exceeds limit")
	}
	return result, nil
}
func lines(ctx context.Context, r io.Reader, consume func([]string) error) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 128*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return err
		}
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if err := consume(strings.Split(line, "\t")); err != nil {
			return err
		}
	}
	if scanner.Err() != nil {
		return errors.New("cannot parse catalog: truncated, corrupt or oversized record")
	}
	return ctx.Err()
}
