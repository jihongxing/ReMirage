package phantom

// Persona 业务画像：每个 Gateway/Cell 绑定的稳定对外身份
type Persona struct {
	CompanyName   string `yaml:"company_name"`
	Domain        string `yaml:"domain"`
	TagLine       string `yaml:"tag_line"`
	PrimaryColor  string `yaml:"primary_color"`
	ErrorPrefix   string `yaml:"error_prefix"`
	APIVersion    string `yaml:"api_version"`
	CopyrightYear int    `yaml:"copyright_year"`
}

// DefaultPersona 默认业务画像
var DefaultPersona = Persona{
	CompanyName:   "CloudBridge Systems",
	Domain:        "cloudbridge.io",
	TagLine:       "Enterprise Cloud Infrastructure",
	PrimaryColor:  "#2563eb",
	ErrorPrefix:   "CB",
	APIVersion:    "v2",
	CopyrightYear: 2026,
}
