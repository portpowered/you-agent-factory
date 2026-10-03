#include "language.h"
#include <iostream>
#include <map>
#include <limits>

using namespace qwen_asr_language;
void require(bool condition, const char* message) {
    if (!condition) throw std::runtime_error(message);
}
std::string decode(const std::vector<int32_t>& ids) {
    const std::map<int32_t, std::string> tokens = {
        {11528, "language"}, {6364, " English"}, {72366, " Canton"},
        {2367, "ese"}, {1, "Hello"}, {2, " world"}, {3, "None"},
        {151704, ""}, {151645, ""}
    };
    std::string text;
    for (const auto id : ids) text += tokens.at(id);
    return text;
}
void prefix_tests() {
    require(languages.size() == 30, "all official languages required");
    for (const auto& language : languages) {
        std::vector<int32_t> prompt{99};
        append(prompt, language.name);
        require(prompt.front() == 99 && prompt[1] == 11528 && prompt.back() == delimiter,
                "prefix must follow assistant prompt");
        require(prompt.size() == language.pieces.size() + 3, "language pieces lost");
    }
    std::vector<int32_t> prompt{99};
    append(prompt, "");
    require(prompt == std::vector<int32_t>{99}, "auto prompt changed");
    append(prompt, "Cantonese");
    require(prompt == std::vector<int32_t>({99,11528,72366,2367,151704}), "Cantonese prefix wrong");
    bool rejected = false;
    try { append(prompt, "Unknown"); } catch (const std::invalid_argument&) { rejected = true; }
    require(rejected, "unknown explicit language accepted");
}
void parsing_tests() {
    auto english = parse({11528,6364,delimiter,1,2}, "", decode);
    require(english.language == "English" && english.text == "Hello world", "auto English corrupted");
    auto cantonese = parse({11528,72366,2367,delimiter,1,2}, "", decode);
    require(cantonese.language == "Cantonese" && cantonese.text == "Hello world", "multi-token language corrupted");
    auto forced = parse({1,2}, "English", decode);
    require(forced.language == "English" && forced.text == "Hello world", "forced first speech tokens lost");
    auto plain = parse({1,2}, "", decode);
    require(plain.language.empty() && plain.text == "Hello world", "delimiter-less speech lost");
    auto short_output = parse({1}, "", decode);
    require(short_output.text == "Hello", "short output lost");
    auto malformed = parse({1,delimiter,2}, "", decode);
    require(malformed.language.empty() && malformed.text == "world", "malformed metadata handling differs");
    require(parse({}, "", decode).text.empty(), "empty output not empty");
    require(parse({151645}, "English", decode).language.empty(), "EOS-only forced silence labelled speech");
    require(parse({11528,3,delimiter}, "", decode).language.empty(), "None detected as language");
}
void digital_silence_tests() {
    const float zeros[] = {0.0f, -0.0f, 0.0f};
    require(exact_digital_silence(zeros, 3), "signed digital zeros must be empty speech");
    require(!exact_digital_silence(zeros, 0), "empty input must retain validation path");
    require(!exact_digital_silence(nullptr, 3), "missing samples are not silence");
    for (const auto nonzero : {std::numeric_limits<float>::denorm_min(),
                              -std::numeric_limits<float>::min(), 0.000001f,
                              std::numeric_limits<float>::infinity(),
                              std::numeric_limits<float>::quiet_NaN()}) {
        const float mixed[] = {0.0f, nonzero, -0.0f};
        require(!exact_digital_silence(mixed, 3), "nonzero or nonfinite audio must reach inference");
        require(exact_digital_silence(mixed, 1), "only the supplied sample range may be inspected");
    }
}
void vocabulary_tests() {
    std::map<int32_t, std::string> pieces(ordinary_pieces.begin(), ordinary_pieces.end());
    pieces[delimiter] = "[PAD151704]";
    const auto piece = [&pieces](int32_t id) { return pieces[id]; };
    validate(151936, piece);
    for (const auto& expected : ordinary_pieces) {
        pieces[expected.first] = "wrong";
        bool rejected = false;
        try { validate(151936, piece); } catch (const std::runtime_error&) { rejected = true; }
        require(rejected, "incompatible ordinary vocabulary accepted");
        pieces[expected.first] = expected.second;
    }
    for (const auto size : {151704, 0}) {
        bool rejected = false;
        try { validate(size, piece); } catch (const std::runtime_error&) { rejected = true; }
        require(rejected, "missing delimiter embedding accepted");
    }
    pieces[delimiter] = "wrong";
    bool rejected = false;
    try { validate(151936, piece); } catch (const std::runtime_error&) { rejected = true; }
    require(rejected, "incompatible delimiter accepted");
}
int main() {
    try {
        prefix_tests(); parsing_tests(); vocabulary_tests(); digital_silence_tests();
        std::cout << "ASR language prefix/parser/vocabulary/digital-silence tests passed\n";
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n'; return 1;
    }
}
