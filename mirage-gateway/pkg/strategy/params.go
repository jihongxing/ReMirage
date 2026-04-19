// Package strategy - 参数转换工具
package strategy

// LevelToParams 导出函数：将防御等级转换为具体参数
func LevelToParams(level DefenseLevel) *DefenseParams {
	return levelToParams(level)
}
