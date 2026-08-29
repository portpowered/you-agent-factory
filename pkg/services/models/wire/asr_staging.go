package wire

import localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"

func bindASRStaging(
	options invocationRuntimeOptions,
	tempDirectory RuntimeTempDirectory,
	tempFile RuntimeCreateTempFile,
	writeFile AssetWriteFile,
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
