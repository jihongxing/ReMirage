package cortex

import "math"

// MarkovModel 马尔可夫链基线模型，用于建模合法流量的时序特征
type MarkovModel struct {
	// TransitionMatrix state → state → probability
	TransitionMatrix map[int]map[int]float64
	// States 离散化的包长区间编号
	// [0,64]=0, [65,256]=1, [257,512]=2, [513,1024]=3, [1025,1500]=4
	States []int
}

// discretize 将包长映射到离散状态
func discretize(size int) int {
	switch {
	case size <= 64:
		return 0
	case size <= 256:
		return 1
	case size <= 512:
		return 2
	case size <= 1024:
		return 3
	default:
		return 4
	}
}

// DefaultBaseline 返回内置的合法流量基线模型
// 基线反映典型 HTTPS/TLS 流量模式：
// - 小包(ACK/控制)之间频繁转移
// - 中包(请求)常跟随大包(响应)
// - 大包(响应)后常回到小包(ACK)
func DefaultBaseline() *MarkovModel {
	states := []int{0, 1, 2, 3, 4}
	tm := map[int]map[int]float64{
		0: {0: 0.30, 1: 0.35, 2: 0.15, 3: 0.10, 4: 0.10},
		1: {0: 0.25, 1: 0.20, 2: 0.25, 3: 0.20, 4: 0.10},
		2: {0: 0.20, 1: 0.15, 2: 0.15, 3: 0.30, 4: 0.20},
		3: {0: 0.30, 1: 0.20, 2: 0.10, 3: 0.15, 4: 0.25},
		4: {0: 0.35, 1: 0.25, 2: 0.15, 3: 0.15, 4: 0.10},
	}
	return &MarkovModel{
		TransitionMatrix: tm,
		States:           states,
	}
}

const maxKL = 10.0

// Deviation 计算观测序列与基线的偏离度（0.0 ~ 1.0）
// 使用 KL 散度归一化：math.Min(klDiv/maxKL, 1.0)
// 空序列或单元素序列返回 0.0
func (m *MarkovModel) Deviation(observed []int) float64 {
	if len(observed) < 2 {
		return 0.0
	}

	// 离散化为状态序列
	seq := make([]int, len(observed))
	for i, size := range observed {
		seq[i] = discretize(size)
	}

	// 统计观测转移计数
	transCount := make(map[int]map[int]int)
	fromCount := make(map[int]int)
	for i := 0; i < len(seq)-1; i++ {
		from, to := seq[i], seq[i+1]
		if transCount[from] == nil {
			transCount[from] = make(map[int]int)
		}
		transCount[from][to]++
		fromCount[from]++
	}

	// 计算观测转移概率矩阵
	obsTM := make(map[int]map[int]float64)
	for from, tos := range transCount {
		obsTM[from] = make(map[int]float64)
		total := float64(fromCount[from])
		for to, count := range tos {
			obsTM[from][to] = float64(count) / total
		}
	}

	// 计算 KL 散度: sum over all (from, to) of obs[from][to] * log(obs[from][to] / baseline[from][to])
	var klDiv float64
	for from, tos := range obsTM {
		baseRow := m.TransitionMatrix[from]
		if baseRow == nil {
			continue
		}
		for to, obsProb := range tos {
			if obsProb <= 0 {
				continue
			}
			baseProb := baseRow[to]
			if baseProb <= 0 {
				// 基线中不存在的转移，使用极小值避免 log(0)
				baseProb = 1e-10
			}
			klDiv += obsProb * math.Log(obsProb/baseProb)
		}
	}

	if klDiv < 0 {
		klDiv = 0
	}

	return math.Min(klDiv/maxKL, 1.0)
}
