/* FEC AVX-512 加速库 - CGO 接口头文件 */

#ifndef __FEC_ACCEL_H__
#define __FEC_ACCEL_H__

#include <stdint.h>

#define FEC_DATA_SHARDS 8
#define FEC_PARITY_SHARDS 4

// 编码：从 data_shards 生成 parity_shards
// data: 连续内存 [data_shards * shard_size]
// parity: 输出连续内存 [parity_shards * shard_size]
// 返回 0 成功，负数失败
int fec_encode(
    const uint8_t *data,
    uint8_t *parity,
    int data_shards,
    int parity_shards,
    int shard_size
);

// 解码：从 available_count 个分片恢复全部 data_shards
// shards: 连续内存 [available_count * shard_size]（按 indices 顺序排列）
// indices: 每个分片的原始索引（0..data_shards-1 为数据，data_shards..total-1 为 parity）
// recovered: 输出连续内存 [data_shards * shard_size]
// 返回 0 成功，负数失败
int fec_decode(
    const uint8_t *shards,
    const int *indices,
    int available_count,
    uint8_t *recovered,
    int data_shards,
    int parity_shards,
    int shard_size
);

// 检测 CPU 是否支持 AVX-512
int fec_has_avx512(void);

#endif /* __FEC_ACCEL_H__ */
