package archivex

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"sort"
	"time"
)

type File struct {
	Name string
	Body []byte
}

type Writer interface {
	WritePackage(dst string, files []File) error
}

type Reader interface {
	ReadPackage(src string) ([]File, error)
}

type ZIPArchive struct{}

func (ZIPArchive) WritePackage(dst string, files []File) (err error) {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := f.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	zw := zip.NewWriter(f)
	defer func() {
		if closeErr := zw.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	ordered := append([]File(nil), files...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].Name < ordered[j].Name
	})

	for _, file := range ordered {
		header := &zip.FileHeader{
			Name:     file.Name,
			Method:   zip.Deflate,
			Modified: time.Unix(0, 0).UTC(),
		}

		writer, err := zw.CreateHeader(header)
		if err != nil {
			return err
		}
		if _, err := writer.Write(file.Body); err != nil {
			return err
		}
	}

	return nil
}

func (ZIPArchive) ReadPackage(src string) ([]File, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}

	readerAt := bytes.NewReader(data)
	zr, err := zip.NewReader(readerAt, int64(len(data)))
	if err != nil {
		return nil, err
	}

	files := make([]File, 0, len(zr.File))
	for _, zipped := range zr.File {
		rc, err := zipped.Open()
		if err != nil {
			return nil, err
		}

		body, readErr := io.ReadAll(rc)
		if readErr != nil {
			_ = rc.Close()
			return nil, readErr
		}
		if closeErr := rc.Close(); closeErr != nil {
			return nil, closeErr
		}

		files = append(files, File{
			Name: zipped.Name,
			Body: body,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return files, nil
}
