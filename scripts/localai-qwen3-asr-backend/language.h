#pragma once
#include <algorithm>
#include <cstdint>
#include <stdexcept>
#include <string>
#include <utility>
#include <vector>

// Official Qwen/Qwen3-ASR-0.6B tokenizer revision
// 5eb144179a02acc5e5ba31e748d22b0cf3e303b0. Generated prefix pieces are
// checked against the loaded GGUF before inference. The converter labels
// the added <asr_text> token [PAD151704], retaining its trained embedding row.
namespace qwen_asr_language {
constexpr int32_t delimiter = 151704;
struct Language { const char* name; std::vector<int32_t> pieces; };
inline const std::vector<Language> languages = {
    {"Chinese", {8453}},
    {"English", {6364}},
    {"Cantonese", {72366, 2367}},
    {"Arabic", {34117}},
    {"German", {5938}},
    {"French", {8585}},
    {"Spanish", {15154}},
    {"Portuguese", {42188}},
    {"Indonesian", {58829}},
    {"Italian", {14811}},
    {"Korean", {16134}},
    {"Russian", {8522}},
    {"Thai", {26392}},
    {"Vietnamese", {48477}},
    {"Japanese", {10769}},
    {"Turkish", {23734}},
    {"Hindi", {43980}},
    {"Malay", {79140}},
    {"Dutch", {23234}},
    {"Swedish", {30109}},
    {"Danish", {43680}},
    {"Finnish", {57853}},
    {"Polish", {31984}},
    {"Czech", {33150}},
    {"Filipino", {62417}},
    {"Persian", {49861}},
    {"Greek", {17860}},
    {"Romanian", {73597}},
    {"Hungarian", {56769}},
    {"Macedonian", {56452, 75491}},
};
inline const std::vector<std::pair<int32_t, std::string>> ordinary_pieces = {
    {2367, "ese"},
    {5938, "ĠGerman"},
    {6364, "ĠEnglish"},
    {8453, "ĠChinese"},
    {8522, "ĠRussian"},
    {8585, "ĠFrench"},
    {10769, "ĠJapanese"},
    {11528, "language"},
    {14811, "ĠItalian"},
    {15154, "ĠSpanish"},
    {16134, "ĠKorean"},
    {17860, "ĠGreek"},
    {23234, "ĠDutch"},
    {23734, "ĠTurkish"},
    {26392, "ĠThai"},
    {30109, "ĠSwedish"},
    {31984, "ĠPolish"},
    {33150, "ĠCzech"},
    {34117, "ĠArabic"},
    {42188, "ĠPortuguese"},
    {43680, "ĠDanish"},
    {43980, "ĠHindi"},
    {48477, "ĠVietnamese"},
    {49861, "ĠPersian"},
    {56452, "ĠMaced"},
    {56769, "ĠHungarian"},
    {57853, "ĠFinnish"},
    {58829, "ĠIndonesian"},
    {62417, "ĠFilipino"},
    {72366, "ĠCanton"},
    {73597, "ĠRomanian"},
    {75491, "onian"},
    {79140, "ĠMalay"},
};
inline std::string trim(std::string value) {
    const auto first = value.find_first_not_of(" \t\r\n");
    return first == std::string::npos ? "" : value.substr(first, value.find_last_not_of(" \t\r\n") - first + 1);
}
inline const Language* find(const std::string& name) {
    for (const auto& language : languages) if (name == language.name) return &language;
    return nullptr;
}
template<class Piece> void validate(int32_t vocabulary_size, Piece piece) {
    if (vocabulary_size <= delimiter) throw std::runtime_error("ASR vocabulary lacks delimiter embedding");
    const auto marker = piece(delimiter);
    if (marker != "<asr_text>" && marker != "[PAD151704]")
        throw std::runtime_error("ASR vocabulary has incompatible delimiter token");
    for (const auto& expected : ordinary_pieces)
        if (piece(expected.first) != expected.second)
            throw std::runtime_error("ASR vocabulary has incompatible language prefix token");
}
inline void append(std::vector<int32_t>& tokens, const std::string& name) {
    if (name.empty()) return;
    const auto* language = find(name);
    if (!language) throw std::invalid_argument("Unsupported explicit ASR language");
    tokens.push_back(11528); // Verified ordinary token: language.
    tokens.insert(tokens.end(), language->pieces.begin(), language->pieces.end());
    tokens.push_back(delimiter);
}
struct Parsed { std::string language, text; };
template<class Decode> Parsed parse(const std::vector<int32_t>& tokens,
                                    const std::string& forced, Decode decode) {
    if (!forced.empty()) {
        const auto text = trim(decode(tokens));
        return {text.empty() ? "" : forced, text};
    }
    const auto marker = std::find(tokens.begin(), tokens.end(), delimiter);
    if (marker == tokens.end()) return {"", trim(decode(tokens))};
    const auto meta = trim(decode(std::vector<int32_t>(tokens.begin(), marker)));
    const auto text = trim(decode(std::vector<int32_t>(marker + 1, tokens.end())));
    const auto language = meta.rfind("language ", 0) == 0 ? trim(meta.substr(9)) : "";
    return {language == "None" ? "" : language, text};
}
} // namespace qwen_asr_language
