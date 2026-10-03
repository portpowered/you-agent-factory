#pragma once

// Split the two-second overlap at its midpoint. Ownership is determined by
// the aligned word's midpoint, so a word crossing the stride seam is retained
// once with its original complete span, rather than cut into two fake spans.
inline bool owns_word(double offset, bool final_window, double start, double end) {
    const double midpoint = offset + (start + end) / 2;
    return (offset == 0 || midpoint >= offset + 1) &&
           (final_window || midpoint < offset + 29);
}
