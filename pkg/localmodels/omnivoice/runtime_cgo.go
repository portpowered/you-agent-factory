//go:build omnivoice_cgo && cgo

// Package omnivoice owns the embedded OmniVoice C ABI integration.
package omnivoice

/*
#cgo CFLAGS: -I${SRCDIR}/../../../third_party/omnivoice-cpp/src
#cgo LDFLAGS: -L${SRCDIR}/../../../native/omnivoice/build -lomnivoice
#include <stdlib.h>
#include <stdio.h>
#include "omnivoice.h"

#if defined(_WIN32)
#include <io.h>
#include <fcntl.h>
static int omnivoice_silence_stderr(void) {
    int saved = _dup(_fileno(stderr));
    if (saved >= 0) { freopen("NUL", "w", stderr); }
    return saved;
}
static void omnivoice_restore_stderr(int saved) {
    if (saved >= 0) { fflush(stderr); _dup2(saved, _fileno(stderr)); _close(saved); }
}
#else
#include <fcntl.h>
#include <unistd.h>
static int omnivoice_silence_stderr(void) {
    int saved = dup(fileno(stderr));
    int null_fd = open("/dev/null", O_WRONLY);
    if (saved >= 0 && null_fd >= 0) { dup2(null_fd, fileno(stderr)); close(null_fd); }
    return saved;
}
static void omnivoice_restore_stderr(int saved) {
    if (saved >= 0) { fflush(stderr); dup2(saved, fileno(stderr)); close(saved); }
}
#endif

static void omnivoice_quiet_log(enum ov_log_level level, const char * message, void * user_data) {
    (void) level;
    (void) message;
    (void) user_data;
}

static void omnivoice_disable_native_logs(void) {
    ov_log_set(omnivoice_quiet_log, NULL);
}
*/
import "C"

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

var disableNativeLogsOnce sync.Once
var nativeStderrMu sync.Mutex

type Model struct {
	mu      sync.Mutex
	context *C.struct_ov_context
}

func Open(modelPath, codecPath string) (*Model, error) {
	disableNativeLogsOnce.Do(func() { C.omnivoice_disable_native_logs() })
	model := C.CString(modelPath)
	defer C.free(unsafe.Pointer(model))
	codec := C.CString(codecPath)
	defer C.free(unsafe.Pointer(codec))

	var params C.struct_ov_init_params
	C.ov_init_default_params(&params)
	params.model_path = model
	params.codec_path = codec
	var context *C.struct_ov_context
	withNativeLogsSilenced(func() { context = C.ov_init(&params) })
	if context == nil {
		return nil, fmt.Errorf("initialize embedded OMNIVOICE runtime: %s", C.GoString(C.ov_last_error()))
	}
	handle := &Model{context: context}
	runtime.SetFinalizer(handle, (*Model).Close)
	return handle, nil
}

func (m *Model) Synthesize(bindings []interfaces.ResolvedModelOperationBinding) ([]float32, int, error) {
	if m == nil || m.context == nil {
		return nil, 0, fmt.Errorf("embedded OMNIVOICE runtime handle is required")
	}
	text, referenceAudio, referenceText, err := invocationInput(bindings)
	if err != nil {
		return nil, 0, err
	}
	textValue := C.CString(text)
	defer C.free(unsafe.Pointer(textValue))
	language := C.CString("en")
	defer C.free(unsafe.Pointer(language))
	var referenceTextValue *C.char
	if referenceText != "" {
		referenceTextValue = C.CString(referenceText)
		defer C.free(unsafe.Pointer(referenceTextValue))
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	var params C.struct_ov_tts_params
	C.ov_tts_default_params(&params)
	params.text = textValue
	params.lang = language
	params.ref_text = referenceTextValue
	if len(referenceAudio) > 0 {
		var voiceRef C.struct_ov_voice_ref
		var status C.enum_ov_status
		withNativeLogsSilenced(func() {
			status = C.ov_extract_voice_ref(m.context, (*C.float)(unsafe.Pointer(unsafe.SliceData(referenceAudio))), C.int(len(referenceAudio)), &voiceRef)
		})
		if status != C.OV_STATUS_OK {
			return nil, 0, fmt.Errorf("extract embedded OMNIVOICE voice reference (%d): %s", int(status), C.GoString(C.ov_last_error()))
		}
		defer C.ov_voice_ref_free(&voiceRef)
		params.ref_audio_tokens = voiceRef.ref_codes
		params.ref_T = voiceRef.ref_T
	}
	var audio C.struct_ov_audio
	var status C.enum_ov_status
	withNativeLogsSilenced(func() { status = C.ov_synthesize(m.context, &params, &audio) })
	if status != C.OV_STATUS_OK {
		return nil, 0, fmt.Errorf("embedded OMNIVOICE synthesis failed (%d): %s", int(status), C.GoString(C.ov_last_error()))
	}
	defer C.ov_audio_free(&audio)
	if audio.samples == nil || audio.n_samples <= 0 || audio.sample_rate <= 0 || audio.channels != 1 {
		return nil, 0, fmt.Errorf("embedded OMNIVOICE returned invalid audio buffer")
	}
	return append([]float32(nil), unsafe.Slice((*float32)(unsafe.Pointer(audio.samples)), int(audio.n_samples))...), int(audio.sample_rate), nil
}

func withNativeLogsSilenced(call func()) {
	nativeStderrMu.Lock()
	defer nativeStderrMu.Unlock()
	saved := C.omnivoice_silence_stderr()
	defer C.omnivoice_restore_stderr(saved)
	call()
}

func (m *Model) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.context != nil {
		C.ov_free(m.context)
		m.context = nil
	}
}

func invocationInput(bindings []interfaces.ResolvedModelOperationBinding) (string, []float32, string, error) {
	var text, audioPath, referenceText string
	for _, binding := range bindings {
		for _, content := range binding.Content {
			switch strings.TrimSpace(binding.Slot) {
			case "text":
				if content.Type.Normalized() == interfaces.WorkContentPartTypeText && strings.TrimSpace(content.Text) != "" {
					text = strings.TrimSpace(content.Text)
				}
			case "reference_audio":
				if content.Type.Normalized() == interfaces.WorkContentPartTypeAudio && strings.TrimSpace(content.File) != "" {
					audioPath = strings.TrimSpace(content.File)
				}
			case "reference_text":
				if content.Type.Normalized() == interfaces.WorkContentPartTypeText && strings.TrimSpace(content.Text) != "" {
					referenceText = strings.TrimSpace(content.Text)
				}
			}
		}
	}
	if text == "" {
		return "", nil, "", fmt.Errorf("embedded OMNIVOICE runtime requires resolved slot %q", "text")
	}
	if audioPath == "" && referenceText == "" {
		return text, nil, "", nil
	}
	if audioPath == "" || referenceText == "" {
		return "", nil, "", fmt.Errorf("embedded OMNIVOICE voice cloning requires both reference_audio and reference_text")
	}
	audio, err := DecodeReferenceWAV(audioPath)
	return text, audio, referenceText, err
}
