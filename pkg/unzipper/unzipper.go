package unzipper

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Unzip takes a source archive and a destination directory and extracts all files
// in the archive to the given destination. If dst does not exist it will attempt
// to create it.
func Unzip(src string, dst string) error {
	err := os.MkdirAll(dst, os.ModePerm)
	if err != nil && os.IsExist(err) {
		return err
	}

	archive, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer archive.Close()

	for _, f := range archive.File {
		filePath := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(filePath, filepath.Clean(dst)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid file path")
		}

		if f.FileInfo().IsDir() {
			err := os.MkdirAll(filePath, os.ModePerm)
			if err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
			return err
		}

		dstFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}

		fileInArchive, err := f.Open()
		if err != nil {
			return err
		}

		if _, err := io.Copy(dstFile, fileInArchive); err != nil {
			return err
		}

		dstFile.Close()
		fileInArchive.Close()
	}

	return nil
}
