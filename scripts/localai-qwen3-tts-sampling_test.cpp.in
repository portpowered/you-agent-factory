// CPU regression for the pinned native Talker and CodePredictor sampler.
#include "sampling.h"
#include <cstdio>
#include <cstdlib>
#include <limits>

static void require(bool condition, const char * label) {
    if (!condition) {
        std::fprintf(stderr, "FAIL: %s\n", label);
        std::exit(1);
    }
}

static int sample(std::vector<float> logits, float temperature = 0.9f,
                  int top_k = 50, float top_p = 1.0f, int64_t seed = 42) {
    return sample_top_k_p(logits.data(), (int) logits.size(), temperature, top_k,
                         top_p, 1.05f, nullptr, 0, seed, 0, nullptr);
}

static void invalid_distributions() {
    const float nan = std::numeric_limits<float>::quiet_NaN();
    for (float temperature : {0.0f, 0.9f}) {
        require(sample({1.0f, nan, -INFINITY}, temperature) == -1, "NaN rejected");
        require(sample({nan, nan}, temperature) == -1, "no finite unmasked candidate rejected");
        require(sample({1.0f, INFINITY}, temperature) == -1, "+Inf rejected");
        require(sample({-INFINITY, -INFINITY}, temperature) == -1, "all masked rejected");
        require(sample({}, temperature) == -1, "empty distribution rejected");
    }
    require(sample({1.0f, -INFINITY}, nan) == -1, "NaN temperature rejected after scaling");
    require(sample({1.0f, -INFINITY}, INFINITY) == -1, "mask times zero rejected");
    require(sample({std::numeric_limits<float>::max(), 1.0f}, 0.5f) == -1,
            "positive overflow after temperature rejected");
    float logits[] = {-std::numeric_limits<float>::max(), -INFINITY};
    int32_t history[] = {0};
    require(sample_top_k_p(logits, 2, 0.9f, 50, 1.0f, 2.0f, history, 1, 42, 0, nullptr) == -1,
            "repetition penalty overflow leaving no finite candidate rejected");
    // Stabilized softmax includes exp(0)=1 for a finite maximum, so zero sum
    // cannot arise from valid logits. Invalid distributions fail before exp.
}

static void finite_policy() {
    require(sample({-INFINITY, 3.0f, 2.0f}, 0.0f) == 1, "finite greedy candidate");
    for (int seed = 0; seed < 128; seed++) {
        require(sample({-INFINITY, 3.0f, 2.0f}, 0.9f, 1, 1.0f, seed) == 1,
                "top-k strongest candidate unchanged");
        require(sample({-INFINITY, 3.0f, 2.0f}, 0.9f, 0, 0.01f, seed) == 1,
                "nucleus strongest candidate unchanged");
        std::vector<float> logits = {-INFINITY, -1.0f, 0.0f, 2.0f};
        float weights[] = {0.0f, expf(-3.0f), expf(-2.0f), 1.0f};
        float sum = 0.0f;
        for (float weight : weights) { sum += weight; }
        float u = 0.0f;
        philox_uniform_fill(seed, 0, 0u, &u, 1);
        float acc = 0.0f;
        int expected = 3;
        for (int i = 0; i < 4; i++) {
            acc += weights[i];
            if (acc >= u * sum) { expected = i; break; }
        }
        require(sample(logits, 1.0f, 0, 1.0f, seed) == expected,
                "finite multinomial and Philox policy unchanged");
    }
    std::vector<float> codec = {-2.0f, 100.0f, 100.0f, 8.0f, 100.0f};
    apply_suppress(codec.data(), (int) codec.size(), 1, 5, 3);
    require(std::isfinite(codec[3]) && codec[1] == -INFINITY, "EOS exemption preserved");
    require(sample(codec, 0.9f, 1) == 3, "EOS remains selectable");
    require(sample({-INFINITY, -10000.0f, -INFINITY}) == 1,
            "stabilized softmax preserves tiny absolute finite logit");
}

int main() {
    invalid_distributions();
    finite_policy();
    std::puts("PASS: invalid distributions, finite sampling policy, and EOS exemption");
    return 0;
}
