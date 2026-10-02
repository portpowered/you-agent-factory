#pragma once
#include <algorithm>
#include <array>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <filesystem>
#include <fstream>
#include <stdexcept>
#include <string>
#include <vector>

// Only the normalized mono 16 kHz WAV edge enters the native models. Read
// bounded windows, retaining exact file duration rather than buffering audio.
class WaveReader {
    std::ifstream file;
    uint64_t data_offset = 0, data_bytes = 0, file_bytes = 0;
    uint16_t format = 0, bytes_per_sample = 0;
    static uint16_t u16(const unsigned char* p) { return p[0] | uint16_t(p[1]) << 8; }
    static uint32_t u32(const unsigned char* p) { return u16(p) | uint32_t(u16(p+2)) << 16; }
    static uint64_t u64(const unsigned char* p) { return u32(p) | uint64_t(u32(p+4)) << 32; }
    void read(unsigned char* out, size_t count) {
        if (!file.read(reinterpret_cast<char*>(out), count)) throw std::runtime_error("Truncated WAV chunk");
    }
    void seek(uint64_t position) {
        if (position > file_bytes) throw std::runtime_error("WAV chunk extends beyond file");
        file.seekg(static_cast<std::streamoff>(position));
        if (!file) throw std::runtime_error("Could not seek WAV file");
    }
public:
    uint64_t frames = 0;
    explicit WaveReader(const std::string& path) : file(std::filesystem::u8path(path), std::ios::binary) {
        if (!file) throw std::runtime_error("Could not open WAV audio");
        file.seekg(0, std::ios::end); file_bytes = static_cast<uint64_t>(file.tellg()); file.seekg(0);
        unsigned char header[12]; read(header, sizeof(header));
        const bool rf64 = std::memcmp(header, "RF64", 4) == 0;
        if ((!rf64 && std::memcmp(header, "RIFF", 4)) || std::memcmp(header+8, "WAVE", 4))
            throw std::runtime_error("Audio is not RIFF/RF64 WAVE");
        uint64_t riff_end = rf64 ? file_bytes : uint64_t(u32(header+4)) + 8;
        if (riff_end > file_bytes || riff_end < 12) throw std::runtime_error("Truncated WAV container");
        bool have_fmt = false, have_data = false, have_ds64 = false;
        uint64_t extended_data_size = 0;
        for (uint64_t position = 12; position < riff_end;) {
            if (riff_end-position < 8) throw std::runtime_error("Truncated WAV chunk header");
            seek(position); unsigned char chunk[8]; read(chunk, 8);
            uint64_t size = u32(chunk+4), body = position+8;
            if (size == UINT32_MAX) {
                if (!rf64 || !have_ds64 || std::memcmp(chunk, "data", 4)) throw std::runtime_error("Unsupported WAV extended chunk");
                size = extended_data_size;
            }
            if (size > riff_end-body) throw std::runtime_error("Truncated WAV chunk payload");
            if (!std::memcmp(chunk, "ds64", 4)) {
                if (!rf64 || size < 28) throw std::runtime_error("Malformed RF64 ds64 chunk");
                unsigned char metadata[28]; read(metadata, 28);
                const uint64_t declared = u64(metadata) + 8;
                if (declared < body+size || declared > file_bytes || u32(metadata+24) > (size-28)/12)
                    throw std::runtime_error("Malformed RF64 size table");
                riff_end = declared; extended_data_size = u64(metadata+8); have_ds64 = true;
            } else if (!std::memcmp(chunk, "fmt ", 4)) {
                if (size < 16) throw std::runtime_error("Malformed WAV format chunk");
                unsigned char fmt[40]{}; read(fmt, static_cast<size_t>(std::min<uint64_t>(size, 40)));
                format = u16(fmt);
                if (format == 0xfffe) {
                    if (size < 40 || u16(fmt+16) < 22) throw std::runtime_error("Malformed extensible WAV format");
                    format = u16(fmt+24);
                }
                const auto bits = u16(fmt+14); bytes_per_sample = bits/8;
                if (u16(fmt+2) != 1 || u32(fmt+4) != 16000 ||
                    !((format == 1 && bits == 16) || (format == 3 && bits == 32)) ||
                    u16(fmt+12) != bytes_per_sample || u32(fmt+8) != 16000*bytes_per_sample)
                    throw std::runtime_error("ASR requires mono 16 kHz PCM16 or float32 WAV");
                have_fmt = true;
            } else if (!std::memcmp(chunk, "data", 4)) {
                if (have_data) throw std::runtime_error("Multiple WAV audio chunks are unsupported");
                data_offset = body; data_bytes = size; have_data = true;
            }
            if (have_fmt && have_data) break;
            const auto padded = size + (size & 1);
            if (padded > riff_end-body) throw std::runtime_error("Missing WAV chunk padding");
            position = body+padded;
        }
        if (!have_fmt || !have_data || (rf64 && !have_ds64) || data_bytes % bytes_per_sample)
            throw std::runtime_error("WAV is missing complete audio frames or metadata");
        frames = data_bytes/bytes_per_sample;
    }
    std::vector<float> window(uint64_t first, size_t maximum) {
        if (first > frames) throw std::runtime_error("WAV window starts beyond audio");
        const auto count = static_cast<size_t>(std::min<uint64_t>(maximum, frames-first));
        std::vector<float> samples(count);
        seek(data_offset+first*bytes_per_sample);
        std::array<unsigned char, 16384> bytes;
        for (size_t offset = 0; offset < count;) {
            const auto batch = std::min(count-offset, bytes.size()/bytes_per_sample);
            read(bytes.data(), batch*bytes_per_sample);
            for (size_t i = 0; i < batch; ++i) {
                if (format == 1) samples[offset+i] = static_cast<int16_t>(u16(bytes.data()+i*2))/32768.0f;
                else {
                    const auto bits = u32(bytes.data()+i*4);
                    std::memcpy(&samples[offset+i], &bits, 4);
                    if (!std::isfinite(samples[offset+i])) throw std::runtime_error("WAV contains non-finite audio samples");
                }
            }
            offset += batch;
        }
        return samples;
    }
};
