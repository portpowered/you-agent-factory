package requestadmission

import "fmt"

type FileSource interface {
	ReadFile(string) ([]byte, error)
}

type RequestFileLoader func(string) (Request, error)
type PayloadFileReader func(string) ([]byte, error)

func NewRequestFileLoader(source FileSource) RequestFileLoader {
	return func(path string) (Request, error) {
		if source == nil {
			return Request{}, fmt.Errorf("Work request file source is required")
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return Request{}, fmt.Errorf("read %s: %w", path, err)
		}
		request, err := ParseCanonicalWorkRequestJSON(data)
		if err != nil {
			return Request{}, fmt.Errorf("parse %s: %w", path, err)
		}
		return request, nil
	}
}

func NewPayloadFileReader(source FileSource) PayloadFileReader {
	return func(path string) ([]byte, error) {
		if source == nil {
			return nil, fmt.Errorf("Work payload file source is required")
		}
		data, err := source.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read payload file: %w", err)
		}
		return data, nil
	}
}
