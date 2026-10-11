package wire

import (
	"context"
	"os"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	localai "github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai"
	"github.com/portpowered/infinite-you/pkg/services/models/internal/backends/localai/videoaudio"
)

type InvocationTempFile = localai.TempFileFactory
type InvocationWriteFile = localai.InputFileWriter
type InvocationRemoveFile = localai.InputFileRemover

func NewInvocationTempFile(next RuntimeCreateTempFile) InvocationTempFile {
	return func(directory, pattern string) (localai.TempFile, error) {
		file, err := next(directory, pattern)
		return file, err
	}
}

func NewInvocationWriteFile(next AssetWriteFile) InvocationWriteFile {
	return func(path string, content []byte) error { return next(path, content, 0o600) }
}

func NewPinnedInvocationASRBackend(dialer InvocationProtocolDialer, temp RuntimeTempDirectory,
	create InvocationTempFile, write InvocationWriteFile, remove InvocationRemoveFile) ASRBackend {
	return localai.NewPinnedASRBackend(dialer, temp, create, write, remove)
}

func NewPinnedInvocationEmbeddingBackend(dialer InvocationProtocolDialer) EmbeddingBackend {
	return localai.NewPinnedEmbeddingBackend(dialer)
}

func NewPinnedInvocationTTSBackend(dialer InvocationProtocolDialer, temp RuntimeTempDirectory,
	create InvocationTempFile, write InvocationWriteFile, inspect RuntimeInspectFile,
	read AssetReadFile, remove InvocationRemoveFile) InvocationTTSBackend {
	return localai.NewPinnedTTSBackend(dialer, temp, create, write,
		func(path string) (os.FileInfo, error) { return inspect(path) }, localai.TTSOutputReader(read), remove)
}

func NewInvocationVideoAudioTransform(runner platformprocess.CommandRunner, temp RuntimeTempDirectory,
	create InvocationTempFile, write InvocationWriteFile, read AssetReadFile, remove InvocationRemoveFile) InvocationAudioTransform {
	return func(ctx context.Context, video []byte) ([]byte, error) {
		return videoaudio.ExtractVideoAudio(ctx, runner, video, temp, create, write, read, remove)
	}
}

func NewInvocationAudioNormalizer(runner platformprocess.CommandRunner, temp RuntimeTempDirectory,
	create InvocationTempFile, write InvocationWriteFile, read AssetReadFile, remove InvocationRemoveFile) InvocationAudioTransform {
	return func(ctx context.Context, audio []byte) ([]byte, error) {
		return videoaudio.NormalizeAudio(ctx, runner, audio, temp, create, write, read, remove)
	}
}
