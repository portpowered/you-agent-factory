package service

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
)

func (service *Service) loadDocument(
	request operatorsettings.LoadDocumentRequest,
) (operatorsettings.LoadDocumentResult, error) {
	if service.files == nil {
		return operatorsettings.LoadDocumentResult{}, fmt.Errorf("operator document filesystem is required")
	}
	if service.decoder == nil {
		return operatorsettings.LoadDocumentResult{}, fmt.Errorf("operator document decoder is required")
	}

	path := strings.TrimSpace(request.Path)
	data, err := service.files.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			if request.RequireExisting {
				return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
					Kind: operatorsettings.DocumentFailureKindNotFound,
					Path: path,
				}
			}
			return operatorsettings.LoadDocumentResult{
				Document: operatorsettings.EmptyDocument,
				Path:     path,
				Found:    false,
			}, nil
		}
		return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindMalformed,
			Message: fmt.Sprintf("read operator document: %v", err),
			Path:    path,
		}
	}

	config, err := service.decoder(data)
	if err != nil {
		return operatorsettings.LoadDocumentResult{}, operatorsettings.DocumentFailure{
			Kind:    operatorsettings.DocumentFailureKindMalformed,
			Message: err.Error(),
			Path:    path,
		}
	}
	return operatorsettings.LoadDocumentResult{
		Document: documentFromConfig(config),
		Path:     path,
		Found:    true,
	}, nil
}
