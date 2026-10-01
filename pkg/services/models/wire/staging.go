package wire

import (
	"os"

	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
)

func bindASRStaging(
	options invocationRuntimeOptions,
	tempDirectory RuntimeTempDirectory,
	tempFile RuntimeCreateTempFile,
	writeFile AssetWriteFile,
	readFile AssetReadFile,
	removeFile AssetRemovePath,
) invocationRuntimeOptions {
	if options.ASRTempDirectory == nil && tempDirectory != nil {
		options.ASRTempDirectory = func() string { return tempDirectory() }
	}
	if options.ASRCreateTemp == nil {
		options.ASRCreateTemp = adaptASRTempFile(tempFile)
	}
	if options.ASRWriteFile == nil && writeFile != nil {
		options.ASRWriteFile = func(path string, content []byte) error {
			return writeFile(path, content, 0o600)
		}
	}
	if options.ASRReadFile == nil && readFile != nil {
		options.ASRReadFile = func(path string) ([]byte, error) { return readFile(path) }
	}
	if options.ASRRemoveFile == nil && removeFile != nil {
		options.ASRRemoveFile = func(path string) error { return removeFile(path) }
	}
	return options
}

func adaptASRTempFile(next RuntimeCreateTempFile) localai.TempFileFactory {
	if next == nil {
		return nil
	}
	return func(directory, pattern string) (localai.TempFile, error) {
		file, err := next(directory, pattern)
		if err != nil || file == nil {
			return localai.TempFile(file), err
		}
		return localai.TempFile(file), nil
	}
}

func bindTTSStaging(
	options invocationRuntimeOptions,
	tempDirectory RuntimeTempDirectory,
	tempFile RuntimeCreateTempFile,
	writeFile AssetWriteFile,
	inspectFile RuntimeInspectFile,
	readFile AssetReadFile,
	removeFile AssetRemovePath,
) invocationRuntimeOptions {
	if options.TTSTempDirectory == nil && tempDirectory != nil {
		options.TTSTempDirectory = func() string { return tempDirectory() }
	}
	if options.TTSCreateTemp == nil {
		options.TTSCreateTemp = adaptTTSTempFile(tempFile)
	}
	if options.TTSWriteFile == nil && writeFile != nil {
		options.TTSWriteFile = func(path string, content []byte) error {
			return writeFile(path, content, 0o600)
		}
	}
	if options.TTSInspectFile == nil && inspectFile != nil {
		options.TTSInspectFile = func(path string) (os.FileInfo, error) {
			return inspectFile(path)
		}
	}
	if options.TTSReadFile == nil && readFile != nil {
		options.TTSReadFile = func(path string) ([]byte, error) { return readFile(path) }
	}
	if options.TTSRemoveFile == nil && removeFile != nil {
		options.TTSRemoveFile = func(path string) error { return removeFile(path) }
	}
	return options
}

func adaptTTSTempFile(next RuntimeCreateTempFile) localai.TempFileFactory {
	if next == nil {
		return nil
	}
	return func(directory, pattern string) (localai.TempFile, error) {
		file, err := next(directory, pattern)
		if err != nil || file == nil {
			return localai.TempFile(file), err
		}
		return localai.TempFile(file), nil
	}
}
