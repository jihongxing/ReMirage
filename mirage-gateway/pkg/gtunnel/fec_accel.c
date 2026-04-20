/* FEC AVX-512 加速库（用户态 C，通过 CGO 调用）
 * Reed-Solomon 编码/解码的 SIMD 加速实现
 * 
 * 编译要求：支持 AVX-512 的 CPU（Skylake-X 及以上）
 * 降级方案：运行时检测 CPU 特性，不支持时回退到 Go 纯软件实现
 */

#include <stdint.h>
#include <string.h>
#include <stdlib.h>

// ============ GF(2^8) 运算（基础版，无 SIMD） ============

// GF(2^8) 不可约多项式: x^8 + x^4 + x^3 + x^2 + 1 = 0x11D
#define GF_POLY 0x1D

// GF(2^8) 乘法（标量）
static inline uint8_t gf_mul(uint8_t a, uint8_t b) {
    uint8_t result = 0;
    for (int i = 0; i < 8; i++) {
        if (b & 1)
            result ^= a;
        uint8_t hi = a & 0x80;
        a <<= 1;
        if (hi)
            a ^= GF_POLY;
        b >>= 1;
    }
    return result;
}

// GF(2^8) 求逆（扩展欧几里得）
static uint8_t gf_inv(uint8_t a) {
    if (a == 0) return 0;
    // a^254 = a^(-1) in GF(2^8)
    uint8_t result = a;
    for (int i = 0; i < 6; i++) {
        result = gf_mul(result, result);
        result = gf_mul(result, a);
    }
    result = gf_mul(result, result);
    return result;
}

// ============ Vandermonde 矩阵 ============

#define FEC_DATA_SHARDS 8
#define FEC_PARITY_SHARDS 4
#define FEC_TOTAL_SHARDS (FEC_DATA_SHARDS + FEC_PARITY_SHARDS)

// 生成 Vandermonde 编码矩阵的 parity 行
// matrix[p][d] = (p+1)^d in GF(2^8)
static void build_encode_matrix(uint8_t matrix[FEC_PARITY_SHARDS][FEC_DATA_SHARDS]) {
    for (int p = 0; p < FEC_PARITY_SHARDS; p++) {
        uint8_t x = (uint8_t)(p + 1); // 生成元 1,2,3,4
        for (int d = 0; d < FEC_DATA_SHARDS; d++) {
            if (d == 0) {
                matrix[p][d] = 1;
            } else {
                matrix[p][d] = gf_mul(matrix[p][d-1], x);
            }
        }
    }
}

// ============ 运行时 CPU 特性检测 ============

#if defined(__x86_64__) || defined(_M_X64)
#include <cpuid.h>

static int has_avx512(void) {
    unsigned int eax, ebx, ecx, edx;
    if (!__get_cpuid_count(7, 0, &eax, &ebx, &ecx, &edx))
        return 0;
    // AVX-512F = bit 16 of EBX
    return (ebx >> 16) & 1;
}
#else
static int has_avx512(void) { return 0; }
#endif

// ============ AVX-512 加速路径 ============

#if defined(__x86_64__) || defined(_M_X64)

#include <immintrin.h>

// AVX-512 GF(2^8) 乘法（64 字节批量）
__attribute__((target("avx512f,avx512bw")))
static void gf_mul_region_avx512(uint8_t *dst, const uint8_t *src, uint8_t coeff, size_t len) {
    if (coeff == 0) {
        memset(dst, 0, len);
        return;
    }
    if (coeff == 1) {
        if (dst != src) memcpy(dst, src, len);
        return;
    }

    // 构建查找表（split 4-bit）
    // low_table[i] = coeff * i for i in 0..15
    // high_table[i] = coeff * (i << 4) for i in 0..15
    uint8_t low_tbl[16], high_tbl[16];
    for (int i = 0; i < 16; i++) {
        low_tbl[i] = gf_mul(coeff, (uint8_t)i);
        high_tbl[i] = gf_mul(coeff, (uint8_t)(i << 4));
    }

    __m512i low_mask = _mm512_set1_epi8(0x0F);

    // 将16字节表广播到512位寄存器的每个128位lane
    __m128i tbl_low_128 = _mm_loadu_si128((__m128i*)low_tbl);
    __m128i tbl_high_128 = _mm_loadu_si128((__m128i*)high_tbl);
    __m512i tbl_low_512 = _mm512_broadcast_i32x4(tbl_low_128);
    __m512i tbl_high_512 = _mm512_broadcast_i32x4(tbl_high_128);

    size_t i = 0;
    for (; i + 64 <= len; i += 64) {
        __m512i data = _mm512_loadu_si512((__m512i*)(src + i));
        __m512i lo = _mm512_and_si512(data, low_mask);
        __m512i hi = _mm512_and_si512(_mm512_srli_epi16(data, 4), low_mask);

        __m512i result_lo = _mm512_shuffle_epi8(tbl_low_512, lo);
        __m512i result_hi = _mm512_shuffle_epi8(tbl_high_512, hi);
        __m512i result = _mm512_xor_si512(result_lo, result_hi);

        _mm512_storeu_si512((__m512i*)(dst + i), result);
    }

    // 处理尾部
    for (; i < len; i++) {
        dst[i] = gf_mul(coeff, src[i]);
    }
}

// AVX-512 XOR（64 字节批量）
__attribute__((target("avx512f,avx512bw")))
static void xor_region_avx512(uint8_t *dst, const uint8_t *src, size_t len) {
    size_t i = 0;
    for (; i + 64 <= len; i += 64) {
        __m512i a = _mm512_loadu_si512((__m512i*)(dst + i));
        __m512i b = _mm512_loadu_si512((__m512i*)(src + i));
        _mm512_storeu_si512((__m512i*)(dst + i), _mm512_xor_si512(a, b));
    }
    for (; i < len; i++) {
        dst[i] ^= src[i];
    }
}

#endif // x86_64

// ============ 标量 fallback ============

static void gf_mul_region_scalar(uint8_t *dst, const uint8_t *src, uint8_t coeff, size_t len) {
    if (coeff == 0) {
        memset(dst, 0, len);
        return;
    }
    if (coeff == 1) {
        if (dst != src) memcpy(dst, src, len);
        return;
    }
    for (size_t i = 0; i < len; i++) {
        dst[i] = gf_mul(coeff, src[i]);
    }
}

static void xor_region_scalar(uint8_t *dst, const uint8_t *src, size_t len) {
    for (size_t i = 0; i < len; i++) {
        dst[i] ^= src[i];
    }
}

// ============ 统一调度接口 ============

static void gf_mul_region(uint8_t *dst, const uint8_t *src, uint8_t coeff, size_t len) {
#if defined(__x86_64__) || defined(_M_X64)
    if (has_avx512()) {
        gf_mul_region_avx512(dst, src, coeff, len);
        return;
    }
#endif
    gf_mul_region_scalar(dst, src, coeff, len);
}

static void xor_region(uint8_t *dst, const uint8_t *src, size_t len) {
#if defined(__x86_64__) || defined(_M_X64)
    if (has_avx512()) {
        xor_region_avx512(dst, src, len);
        return;
    }
#endif
    xor_region_scalar(dst, src, len);
}

// ============ 导出 API（CGO 调用） ============

// fec_encode: 从 data_shards 生成 parity_shards
// data: 连续内存 [data_shards * shard_size]
// parity: 输出连续内存 [parity_shards * shard_size]
int fec_encode(
    const uint8_t *data,
    uint8_t *parity,
    int data_shards,
    int parity_shards,
    int shard_size
) {
    if (data_shards != FEC_DATA_SHARDS || parity_shards != FEC_PARITY_SHARDS)
        return -1;

    uint8_t matrix[FEC_PARITY_SHARDS][FEC_DATA_SHARDS];
    build_encode_matrix(matrix);

    // 清零 parity
    memset(parity, 0, (size_t)parity_shards * shard_size);

    uint8_t *tmp = (uint8_t*)malloc(shard_size);
    if (!tmp) return -2;

    for (int p = 0; p < parity_shards; p++) {
        uint8_t *parity_shard = parity + p * shard_size;

        for (int d = 0; d < data_shards; d++) {
            const uint8_t *data_shard = data + d * shard_size;
            uint8_t coeff = matrix[p][d];

            gf_mul_region(tmp, data_shard, coeff, shard_size);
            xor_region(parity_shard, tmp, shard_size);
        }
    }

    free(tmp);
    return 0;
}

// fec_decode: 从 available_count 个分片恢复全部 data_shards
// shards: 连续内存 [available_count * shard_size]
// indices: 每个分片的原始索引
// recovered: 输出连续内存 [data_shards * shard_size]
int fec_decode(
    const uint8_t *shards,
    const int *indices,
    int available_count,
    uint8_t *recovered,
    int data_shards,
    int parity_shards,
    int shard_size
) {
    if (data_shards != FEC_DATA_SHARDS || parity_shards != FEC_PARITY_SHARDS)
        return -1;
    if (available_count < data_shards)
        return -3; // 分片不足

    // 构建完整编码矩阵（单位矩阵 + Vandermonde parity 行）
    uint8_t full_matrix[FEC_TOTAL_SHARDS][FEC_DATA_SHARDS];
    // 上半部分：单位矩阵（数据分片）
    memset(full_matrix, 0, sizeof(full_matrix));
    for (int i = 0; i < FEC_DATA_SHARDS; i++)
        full_matrix[i][i] = 1;
    // 下半部分：Vandermonde parity 行
    uint8_t parity_matrix[FEC_PARITY_SHARDS][FEC_DATA_SHARDS];
    build_encode_matrix(parity_matrix);
    for (int p = 0; p < FEC_PARITY_SHARDS; p++)
        memcpy(full_matrix[FEC_DATA_SHARDS + p], parity_matrix[p], FEC_DATA_SHARDS);

    // 提取子矩阵（只取 available 的行）
    uint8_t sub_matrix[FEC_DATA_SHARDS][FEC_DATA_SHARDS];
    for (int i = 0; i < data_shards; i++) {
        memcpy(sub_matrix[i], full_matrix[indices[i]], FEC_DATA_SHARDS);
    }

    // 高斯消元求逆矩阵
    uint8_t inv_matrix[FEC_DATA_SHARDS][FEC_DATA_SHARDS];
    memset(inv_matrix, 0, sizeof(inv_matrix));
    for (int i = 0; i < data_shards; i++)
        inv_matrix[i][i] = 1;

    for (int col = 0; col < data_shards; col++) {
        // 找主元
        int pivot = -1;
        for (int row = col; row < data_shards; row++) {
            if (sub_matrix[row][col] != 0) {
                pivot = row;
                break;
            }
        }
        if (pivot < 0) return -4; // 矩阵奇异

        // 交换行
        if (pivot != col) {
            uint8_t tmp_row[FEC_DATA_SHARDS];
            memcpy(tmp_row, sub_matrix[col], FEC_DATA_SHARDS);
            memcpy(sub_matrix[col], sub_matrix[pivot], FEC_DATA_SHARDS);
            memcpy(sub_matrix[pivot], tmp_row, FEC_DATA_SHARDS);
            memcpy(tmp_row, inv_matrix[col], FEC_DATA_SHARDS);
            memcpy(inv_matrix[col], inv_matrix[pivot], FEC_DATA_SHARDS);
            memcpy(inv_matrix[pivot], tmp_row, FEC_DATA_SHARDS);
        }

        // 归一化主元行
        uint8_t inv_pivot = gf_inv(sub_matrix[col][col]);
        for (int j = 0; j < data_shards; j++) {
            sub_matrix[col][j] = gf_mul(sub_matrix[col][j], inv_pivot);
            inv_matrix[col][j] = gf_mul(inv_matrix[col][j], inv_pivot);
        }

        // 消元
        for (int row = 0; row < data_shards; row++) {
            if (row == col) continue;
            uint8_t factor = sub_matrix[row][col];
            if (factor == 0) continue;
            for (int j = 0; j < data_shards; j++) {
                sub_matrix[row][j] ^= gf_mul(factor, sub_matrix[col][j]);
                inv_matrix[row][j] ^= gf_mul(factor, inv_matrix[col][j]);
            }
        }
    }

    // 用逆矩阵恢复数据
    uint8_t *tmp = (uint8_t*)malloc(shard_size);
    if (!tmp) return -2;

    memset(recovered, 0, (size_t)data_shards * shard_size);

    for (int d = 0; d < data_shards; d++) {
        uint8_t *out_shard = recovered + d * shard_size;

        for (int i = 0; i < data_shards; i++) {
            uint8_t coeff = inv_matrix[d][i];
            if (coeff == 0) continue;

            const uint8_t *in_shard = shards + i * shard_size;
            gf_mul_region(tmp, in_shard, coeff, shard_size);
            xor_region(out_shard, tmp, shard_size);
        }
    }

    free(tmp);
    return 0;
}

// fec_has_avx512: 返回是否支持 AVX-512
int fec_has_avx512(void) {
    return has_avx512();
}
