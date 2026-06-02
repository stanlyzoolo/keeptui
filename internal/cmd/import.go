package cmd

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lepeshko/keys/internal/loader"
)

func RunImport(src string) error {
	var data []byte
	var err error

	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		data, err = fetchURL(src)
	} else {
		data, err = os.ReadFile(src)
	}
	if err != nil {
		return fmt.Errorf("cannot read source: %v", err)
	}

	tool, errs := loader.Validate(data)
	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "Validation failed:")
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  ERROR: %s\n", e)
		}
		return fmt.Errorf("fix errors above and retry")
	}

	dest, err := userToolPath(tool.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return fmt.Errorf("cannot create config dir: %v", err)
	}

	if _, err := os.Stat(dest); err == nil {
		ok, err := confirmOverwrite(dest)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("cannot write file: %v", err)
	}

	fmt.Printf("Imported %q → %s\n", tool.Name, dest)
	return nil
}

func fetchURL(url string) ([]byte, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url) //nolint:gosec
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
