package gtclient

import (
	"encoding/json"
	"fmt"
	"os"
)

// TopoCache 路由表本地持久化缓存
type TopoCache struct {
	path string // 本地缓存文件路径
}

// NewTopoCache 创建拓扑缓存
func NewTopoCache(path string) *TopoCache {
	return &TopoCache{path: path}
}

// Save 将路由表序列化后写入本地文件
func (tc *TopoCache) Save(resp *RouteTableResponse) error {
	if tc.path == "" {
		return fmt.Errorf("cache path not set")
	}
	data, err := json.Marshal(resp)
	if err != nil {
		return fmt.Errorf("marshal route table: %w", err)
	}
	return os.WriteFile(tc.path, data, 0600)
}

// Load 从文件读取并反序列化路由表
func (tc *TopoCache) Load() (*RouteTableResponse, error) {
	if tc.path == "" {
		return nil, fmt.Errorf("cache path not set")
	}
	data, err := os.ReadFile(tc.path)
	if err != nil {
		return nil, fmt.Errorf("read cache: %w", err)
	}
	var resp RouteTableResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, fmt.Errorf("unmarshal cache: %w", err)
	}
	return &resp, nil
}
