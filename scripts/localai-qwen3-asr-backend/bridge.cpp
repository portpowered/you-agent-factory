#include "qwen3_asr.h"
#include "forced_aligner.h"
#include <algorithm>
#include <cctype>
#include <cmath>
#include <cstdint>
#include <limits>
#include <memory>
#include <stdexcept>
#include "seam.h"

#define API extern "C" __declspec(dllexport)
namespace {
std::unique_ptr<qwen3_asr::Qwen3ASR> asr;
std::unique_ptr<qwen3_asr::ForcedAligner> aligner;
std::string error, text, language;
double duration;
struct Word { std::string text; int64_t start, end; };
std::vector<Word> words;

std::string transcript(std::string value) {
    auto marker = value.find("<asr_text>");
    if (marker != std::string::npos) return value.substr(marker + 10);
    if (value.rfind("language ", 0) == 0) {
        auto separator = value.find(' ', 9);
        if (separator != std::string::npos) return value.substr(separator + 1);
    }
    return value;
}
}

API const char* qa_error() { return error.c_str(); }
API int qa_load(const char* model, const char* alignment, const char* dictionary) {
    try {
        asr.reset(); aligner.reset();
        auto next_asr = std::make_unique<qwen3_asr::Qwen3ASR>();
        auto next_aligner = std::make_unique<qwen3_asr::ForcedAligner>();
        if (!next_asr->load_model(model)) throw std::runtime_error(next_asr->get_error());
        if (!next_aligner->load_model(alignment)) throw std::runtime_error(next_aligner->get_error());
        if (dictionary && *dictionary && !next_aligner->load_korean_dict(dictionary))
            throw std::runtime_error("Could not load the bundled Korean alignment dictionary");
        asr = std::move(next_asr); aligner = std::move(next_aligner);
        error.clear(); return 0;
    } catch (const std::exception& e) { error = e.what(); return 1; }
}

API int qa_transcribe(const char* path, const char* requested_language, int threads) {
    try {
        if (!asr || !aligner) throw std::runtime_error("ASR and forced aligner are not loaded");
        words.clear(); text.clear(); language.clear();
        std::vector<float> samples; int rate = 0;
        if (!qwen3_asr::load_audio_file(path, samples, rate) || rate != 16000)
            throw std::runtime_error("ASR requires a readable mono 16 kHz WAV");
        duration = static_cast<double>(samples.size()) / rate;
        // Match the pinned upstream CLI's overlapping windows. No input-duration
        // ceiling: align each window and translate its real timestamps globally.
        constexpr size_t window = 30 * 16000, stride = 28 * 16000;
        for (size_t offset = 0; offset < samples.size(); offset += stride) {
            const auto count = std::min(window, samples.size() - offset);
            qwen3_asr::transcribe_params params;
            params.language = requested_language;
            params.n_threads = threads > 0 ? threads : 4;
            params.print_timing = false;
            qwen3_asr::transcribe_result decoded;
            for (;;) {
                decoded = asr->transcribe(samples.data() + offset, static_cast<int>(count), params);
                if (!decoded.success) throw std::runtime_error(decoded.error_msg);
                if (decoded.tokens.size() < static_cast<size_t>(params.max_tokens)) break;
                if (params.max_tokens > std::numeric_limits<int32_t>::max() / 2)
                    throw std::runtime_error("ASR decoder context cannot grow further");
                params.max_tokens *= 2;
            }
            const auto recognized = transcript(decoded.text);
            if (language.empty()) {
                language = decoded.language;
                auto first = language.find_first_not_of(" \t\r\n");
                language = first == std::string::npos ? "" : language.substr(first, language.find_last_not_of(" \t\r\n") - first + 1);
            }
            if (recognized.find_first_not_of(" \t\r\n") != std::string::npos) {
                auto lang = decoded.language.empty() ? std::string(requested_language) : decoded.language;
                std::transform(lang.begin(), lang.end(), lang.begin(), [](unsigned char c) { return std::tolower(c); });
                aligner->set_n_threads(params.n_threads);
                auto aligned = aligner->align(samples.data() + offset, static_cast<int>(count), recognized, lang);
                if (!aligned.success) throw std::runtime_error(aligned.error_msg);
                if (aligned.words.empty()) throw std::runtime_error("Forced aligner returned no words for speech");
                for (const auto& word : aligned.words) {
                    if (!std::isfinite(word.start) || !std::isfinite(word.end) || word.start < 0 || word.end < word.start || word.end > static_cast<double>(count) / rate + 0.001)
                        throw std::runtime_error("Forced aligner returned timestamps outside its audio window");
                    if (!owns_word(offset / 16000.0, offset + count == samples.size(), word.start, word.end)) continue;
                    // The aligner predicts 80 ms classes. Round float seconds to
                    // its integer millisecond evidence before encoding nanos;
                    // otherwise float 1.6 becomes 1.599999905 and loses 1 ms.
                    const auto base = static_cast<int64_t>(offset / rate) * 1000000000;
                    const auto start = base + std::llround(word.start * 1000.0) * 1000000;
                    const auto end = base + std::llround(word.end * 1000.0) * 1000000;
                    if (!words.empty() && start < words.back().end) {
                        if (word.word == words.back().text) continue;
                        throw std::runtime_error("Forced alignment contains conflicting overlapping word spans");
                    }
                    words.push_back({word.word, start, end});
                }
            }
            if (offset + count == samples.size()) break;
        }
        auto normalized = language;
        std::transform(normalized.begin(), normalized.end(), normalized.begin(), [](unsigned char c) { return std::tolower(c); });
        const bool spaces = normalized != "chinese" && normalized != "japanese" && normalized != "cantonese";
        for (const auto& word : words) { if (spaces && !text.empty()) text += " "; text += word.text; }
        error.clear(); return 0;
    } catch (const std::exception& e) { error = e.what(); return 1; }
}
API const char* qa_text() { return text.c_str(); }
API const char* qa_language() { return language.c_str(); }
API double qa_duration() { return duration; }
API int64_t qa_word_count() { return static_cast<int64_t>(words.size()); }
API const char* qa_word_text(int64_t index) { return words.at(index).text.c_str(); }
API int64_t qa_word_start(int64_t index) { return words.at(index).start; }
API int64_t qa_word_end(int64_t index) { return words.at(index).end; }
