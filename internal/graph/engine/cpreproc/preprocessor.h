#ifndef SO_PREPROCESSOR_H
#define SO_PREPROCESSOR_H

#include <stddef.h>
#include <stdint.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct {
	char *source;
	uint32_t *original_line_by_expanded_line; // 1-based; 0 means directive/unmapped.
	uint8_t *belongs_to_main_file;            // 1-based; true only for the original input file.
	int expanded_line_count;
} SOPreprocessedSource;

// Expand macros / #ifdef / #include. Returns NULL if no work or on failure.
// extra_defines and include_paths are NULL-terminated C string arrays (may be NULL).
SOPreprocessedSource *so_preprocess_with_map(const char *source, int source_len, const char *filename,
					     const char **extra_defines, const char **include_paths,
					     int cpp_mode);

void so_preprocessed_source_free(SOPreprocessedSource *pp);

#ifdef __cplusplus
}
#endif

#endif
