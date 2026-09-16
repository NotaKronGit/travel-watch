// devcerts writes ignored env files for local gRPC/mTLS. It never prints secrets.
package main

import (
	"errors"
	"fmt"
	"github.com/NotaKronGit/travel-watch/api/mtls"
	"github.com/NotaKronGit/travel-watch/internal/devpki"
	"os"
	"path/filepath"
)

func run() error {
	dir := ".local/tls"
	aExists, bExists := false, false
	if _, err := os.Stat(filepath.Join(dir, "search.env")); err == nil {
		aExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if _, err := os.Stat(filepath.Join(dir, "cabinet.env")); err == nil {
		bExists = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if aExists && bExists {
		fmt.Println("Local TLS env files already exist; left unchanged.")
		return nil
	}
	if aExists || bExists {
		return errors.New("partial TLS environment exists; use a fresh directory for regeneration")
	}
	ca, err := devpki.New()
	if err != nil {
		return err
	}
	server, err := ca.Issue("search", "cabinet", true)
	if err != nil {
		return err
	}
	client, err := ca.Issue("cabinet", "search", false)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	entries := []struct {
		name, prefix, extra string
		cfg                 mtls.Config
	}{
		{"search.env", "SEARCH_RESULTS_TLS_", "", server},
		{"cabinet.env", "CABINET_SEARCH_TLS_", "CABINET_SEARCH_ENABLED=true\n", client},
	}
	for _, e := range entries {
		f, err := os.OpenFile(filepath.Join(dir, e.name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(f, "%s%sCA_BASE64=%s\n%sCERT_BASE64=%s\n%sKEY_BASE64=%s\n%sPEER_NAME=%s\n", e.extra, e.prefix, e.cfg.CABase64, e.prefix, e.cfg.CertBase64, e.prefix, e.cfg.KeyBase64, e.prefix, e.cfg.PeerName)
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	fmt.Println("Created .local/tls/search.env and cabinet.env (local certificates, valid one month).")
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Cannot generate local TLS environment:", err)
		os.Exit(1)
	}
}
