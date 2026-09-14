package catalog

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
)

type localizedName struct {
	name             string
	preferred, short bool
	id               int64
}

func (n localizedName) better(old localizedName) bool {
	if old.name == "" {
		return true
	}
	if n.preferred != old.preferred {
		return n.preferred
	}
	if n.short != old.short {
		return n.short
	}
	return n.id < old.id
}

func (s GeoNamesSource) localize(ctx context.Context, snapshot *Snapshot, digest hash.Hash) error {
	if s.AlternateNamesURL == "" || s.MaxAlternateDownloadBytes <= 0 || s.MaxAlternateUncompressedBytes <= 0 {
		return errors.New("invalid alternate names configuration")
	}
	// ZIP requires ReaderAt. Keep its large compressed payload on disk, never
	// extract paths supplied by the archive, and always remove the temporary file.
	file, err := os.CreateTemp("", "travel-watch-names-*.zip")
	if err != nil {
		return errors.New("cannot create alternate names temporary file")
	}
	defer os.Remove(file.Name())
	defer file.Close()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.AlternateNamesURL, nil)
	if err != nil {
		return errors.New("invalid alternate names URL")
	}
	req.Header.Set("User-Agent", "TravelWatch-catalog-importer")
	res, err := s.Client.Do(req)
	if err != nil {
		return errors.New("alternate names download failed or timed out")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("alternate names HTTP status %d", res.StatusCode)
	}
	size, err := io.Copy(io.MultiWriter(file, digest), io.LimitReader(res.Body, s.MaxAlternateDownloadBytes+1))
	if err != nil {
		return errors.New("cannot download alternate names")
	}
	if size > s.MaxAlternateDownloadBytes {
		return errors.New("alternate names download exceeds limit")
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return errors.New("invalid alternate names ZIP")
	}
	var entry *zip.File
	for _, f := range archive.File {
		if f.Name == "alternateNamesV2.txt" {
			if entry != nil {
				return errors.New("duplicate alternate names ZIP entry")
			}
			entry = f
		}
	}
	if entry == nil || entry.FileInfo().IsDir() {
		return errors.New("alternateNamesV2.txt missing in ZIP")
	}
	if entry.UncompressedSize64 > uint64(s.MaxAlternateUncompressedBytes) {
		return errors.New("uncompressed alternate names exceed limit")
	}
	reader, err := entry.Open()
	if err != nil {
		return errors.New("cannot open alternate names ZIP entry")
	}
	defer reader.Close()
	limited := &io.LimitedReader{R: reader, N: s.MaxAlternateUncompressedBytes + 1}
	if err := applyRussianNames(ctx, snapshot, limited); err != nil {
		return err
	}
	if limited.N <= 0 {
		return errors.New("uncompressed alternate names exceed limit")
	}
	return nil
}

func applyRussianNames(ctx context.Context, snapshot *Snapshot, reader io.Reader) error {
	cities := make(map[int64]int, len(snapshot.Cities))
	countries := make(map[int64]int, len(snapshot.Countries))
	for i, c := range snapshot.Cities {
		cities[c.SourceID] = i
	}
	for i, c := range snapshot.Countries {
		if c.SourceID > 0 {
			countries[c.SourceID] = i
		}
	}
	chosen := make(map[int64]localizedName)
	matches := 0
	err := lines(ctx, reader, func(f []string) error {
		if len(f) != 10 {
			return errors.New("invalid alternateNamesV2 record")
		}
		if f[2] != "ru" {
			return nil
		}
		id, err := strconv.ParseInt(f[1], 10, 64)
		if err != nil || id <= 0 {
			return errors.New("invalid localized GeoNames ID")
		}
		ci, city := cities[id]
		_, country := countries[id]
		if !city && !country {
			return nil
		}
		alternateID, err := strconv.ParseInt(f[0], 10, 64)
		if err != nil || alternateID <= 0 || !validText(f[3], 400) {
			return errors.New("invalid Russian name record")
		}
		for _, flag := range f[4:8] {
			if flag != "" && flag != "0" && flag != "1" {
				return errors.New("invalid alternate name flag")
			}
		}
		// Dated, historic and colloquial names are not display-name candidates.
		if f[6] == "1" || f[7] == "1" || f[8] != "" || f[9] != "" {
			return nil
		}
		candidate := localizedName{name: f[3], preferred: f[4] == "1", short: f[5] == "1", id: alternateID}
		if candidate.better(chosen[id]) {
			chosen[id] = candidate
		}
		if city && f[3] != snapshot.Cities[ci].Name && !slices.Contains(snapshot.Cities[ci].Aliases, f[3]) {
			snapshot.Cities[ci].Aliases = append(snapshot.Cities[ci].Aliases, f[3])
		}
		matches++
		return nil
	})
	if err != nil {
		return err
	}
	if matches == 0 {
		return errors.New("alternate names contain no matching Russian names")
	}
	for id, n := range chosen {
		if i, ok := cities[id]; ok {
			snapshot.Cities[i].NameRu = n.name
		}
		if i, ok := countries[id]; ok {
			snapshot.Countries[i].NameRu = n.name
		}
	}
	return nil
}
