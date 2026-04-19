/* G-Tunnel FEC 内核加速头文件
 * AVX-512 优化的 Reed-Solomon 编码
 */

#ifndef __FEC_KERNEL_H__
#define __FEC_KERNEL_H__

#include <stdint.h>
#include <immintrin.h>

// FEC 配置
#define FEC_DATA_SHARDS 8      // 数据分片数
#define FEC_PARITY_SHARDS 4    // 冗余分片数
#define FEC_SHARD_SIZE 1024    // 每个分片大小（字节）

// AVX-512 加速的 Galois Field 乘法
static inline __m512i gf_mul_avx512(__m512i a, uint8_t b) {
    // 使用 AVX-512 指令集加速 GF(2^8) 乘法
    // 这是 Reed-Solomon 编码的核心运算
    __m512i result = _mm512_setzero_si512();
    __m512i mask = _mm512_set1_epi8(0x01);
    
    for (int i = 0; i < 8; i++) {
        __m512i temp = _mm512_and_si512(a, mask);
        __mmask64 cmp = _mm512_test_epi8_mask(temp, temp);
        result = _mm512_mask_xor_epi8(result, cmp, result, _mm512_set1_epi8(b));
        
        b = (b << 1) ^ ((b & 0x80) ? 0x1D : 0);
        mask = _mm512_slli_epi16(mask, 1);
    }
    
    return result;
}

// FEC 编码（生成冗余分片）
static inline void fec_encode_avx512(
    uint8_t *data_shards[FEC_DATA_SHARDS],
    uint8_t *parity_shards[FEC_PARITY_SHARDS],
    size_t shard_size
) {
    // 使用 Vandermonde 矩阵生成冗余
    for (int p = 0; p < FEC_PARITY_SHARDS; p++) {
        __m512i parity = _mm512_setzero_si512();
        
        for (int d = 0; d < FEC_DATA_SHARDS; d++) {
            // 计算 Vandermonde 矩阵元素
            uint8_t coeff = 1;
            for (int k = 0; k < d; k++) {
                coeff = (coeff * (p + FEC_DATA_SHARDS)) % 255;
            }
            
            // AVX-512 批量处理 64 字节
            for (size_t i = 0; i < shard_size; i += 64) {
                __m512i data = _mm512_loadu_si512((__m512i*)(data_shards[d] + i));
                __m512i prod = gf_mul_avx512(data, coeff);
                parity = _mm512_xor_si512(parity, prod);
            }
        }
        
        // 写入冗余分片
        for (size_t i = 0; i < shard_size; i += 64) {
            _mm512_storeu_si512((__m512i*)(parity_shards[p] + i), parity);
        }
    }
}

// FEC 解码（恢复丢失分片）
static inline int fec_decode_avx512(
    uint8_t *shards[FEC_DATA_SHARDS + FEC_PARITY_SHARDS],
    uint8_t *recovered[FEC_DATA_SHARDS],
    int *available_indices,
    int available_count,
    size_t shard_size
) {
    if (available_count < FEC_DATA_SHARDS) {
        return -1; // 无法恢复
    }
    
    // 使用高斯消元法恢复丢失的分片
    // 这里简化实现，实际需要完整的矩阵求逆
    for (int i = 0; i < FEC_DATA_SHARDS; i++) {
        if (available_indices[i] < FEC_DATA_SHARDS) {
            // 数据分片完好，直接复制
            for (size_t j = 0; j < shard_size; j += 64) {
                __m512i data = _mm512_loadu_si512((__m512i*)(shards[available_indices[i]] + j));
                _mm512_storeu_si512((__m512i*)(recovered[i] + j), data);
            }
        }
    }
    
    return 0;
}

#endif /* __FEC_KERNEL_H__ */
