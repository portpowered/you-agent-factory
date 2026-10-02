#include "seam.h"
#include <cstdlib>

int main() {
    // A complete word crossing the old 28-second stride must not be repeated.
    if (!owns_word(0, false, 27.9, 28.3) || owns_word(28, false, 0, .3)) return EXIT_FAILURE;
    // A word straddling the overlap midpoint belongs only to the next window.
    if (owns_word(0, false, 28.9, 29.3) || !owns_word(28, true, .9, 1.3)) return EXIT_FAILURE;
    // Initial and final edges retain aligned spans without duration fabrication.
    if (!owns_word(0, true, 0, .4) || !owns_word(28, true, 8, 9)) return EXIT_FAILURE;
}
