package threat

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"sort"
	"sync"
	"time"

	"mirage-gateway/pkg/ebpf"
)

// BlacklistManager 黑名单管理器
type BlacklistManager struct {
	entries    map[string]*BlacklistEntry // CIDR → entry
	loader     *ebpf.Loader
	mu         sync.RWMutex
	maxEntries int
}

// NewBlacklistManager 创建黑名单管理器
func NewBlacklistManager(loader *ebpf.Loader, maxEntries int) *BlacklistManager {
	return &BlacklistManager{
		entries:    make(map[string]*BlacklistEntry),
		loader:     loader,
		maxEntries: maxEntries,
	}
}

// Add 添加条目，1 秒内同步到 eBPF LPM Trie Map
func (bm *BlacklistManager) Add(cidr string, expireAt time.Time, source BlacklistSource) error {
	_, _, err := net.ParseCIDR(cidr)
	if err != nil {
		// 尝试作为单个 IP
		ip := net.ParseIP(cidr)
		if ip == nil {
			return fmt.Errorf("无效的 CIDR 或 IP: %s", cidr)
		}
		cidr = cidr + "/32"
	}

	bm.mu.Lock()
	defer bm.mu.Unlock()

	// 容量检查
	if len(bm.entries) >= bm.maxEntries {
		bm.evictOldest()
	}

	bm.entries[cidr] = &BlacklistEntry{
		CIDR:     cidr,
		AddedAt:  time.Now(),
		ExpireAt: expireAt,
		Source:   source,
	}

	// 同步到 eBPF
	bm.syncToEBPF(cidr, true)

	return nil
}

// Remove 移除条目
func (bm *BlacklistManager) Remove(cidr string) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if _, ok := bm.entries[cidr]; !ok {
		return fmt.Errorf("条目不存在: %s", cidr)
	}

	delete(bm.entries, cidr)
	bm.syncToEBPF(cidr, false)
	return nil
}

// MergeGlobal 合并 OS 下发的全局黑名单（全局优先级高于本地）
func (bm *BlacklistManager) MergeGlobal(entries []BlacklistEntry) error {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for _, entry := range entries {
		entry.Source = SourceGlobal
		e := entry // copy
		bm.entries[entry.CIDR] = &e
		bm.syncToEBPF(entry.CIDR, true)
	}

	// 容量淘汰
	for len(bm.entries) > bm.maxEntries {
		bm.evictOldest()
	}

	return nil
}

// StartExpiry 启动过期清理循环
func (bm *BlacklistManager) StartExpiry(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				bm.cleanExpired()
			}
		}
	}()
	log.Println("[Blacklist] 过期清理循环已启动")
}

// Count 获取当前条目数
func (bm *BlacklistManager) Count() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return len(bm.entries)
}

// Get 获取条目（供测试使用）
func (bm *BlacklistManager) Get(cidr string) *BlacklistEntry {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.entries[cidr]
}

// cleanExpired 清理过期条目
func (bm *BlacklistManager) cleanExpired() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	now := time.Now()
	for cidr, entry := range bm.entries {
		if !entry.ExpireAt.IsZero() && now.After(entry.ExpireAt) {
			delete(bm.entries, cidr)
			bm.syncToEBPF(cidr, false)
		}
	}
}

// evictOldest 淘汰最早过期的条目
func (bm *BlacklistManager) evictOldest() {
	if len(bm.entries) == 0 {
		return
	}

	type kv struct {
		cidr   string
		expire time.Time
	}
	var all []kv
	for cidr, entry := range bm.entries {
		all = append(all, kv{cidr, entry.ExpireAt})
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].expire.Before(all[j].expire)
	})

	victim := all[0].cidr
	delete(bm.entries, victim)
	bm.syncToEBPF(victim, false)
}

// syncToEBPF 同步到 eBPF LPM Trie Map
func (bm *BlacklistManager) syncToEBPF(cidr string, add bool) {
	if bm.loader == nil {
		return
	}

	lpmMap := bm.loader.GetMap("blacklist_lpm")
	if lpmMap == nil {
		return
	}

	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}

	// LPM Trie key: prefixlen(4 bytes) + ip(4 bytes)
	ones, _ := ipNet.Mask.Size()
	key := make([]byte, 8)
	binary.LittleEndian.PutUint32(key[0:4], uint32(ones))
	copy(key[4:8], ipNet.IP.To4())

	if add {
		value := uint32(1)
		if err := lpmMap.Put(key, &value); err != nil {
			log.Printf("[Blacklist] eBPF LPM 写入失败: %v", err)
		}
	} else {
		if err := lpmMap.Delete(key); err != nil {
			// 删除失败不报错（可能已不存在）
		}
	}
}
