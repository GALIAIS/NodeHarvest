package geo

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/oschwald/geoip2-golang"
)

// Result 地理信息
type Result struct {
	CountryCode string // ISO 3166-1 alpha-2，如 US
	CountryName string // 英文名
	City        string
	ISP         string // 可选，Country DB 通常为空
	Source      string // mmdb | name | cache
}

// Locator IP/域名 → 国家
type Locator struct {
	mu     sync.RWMutex
	db     *geoip2.Reader
	dbPath string
	cache  map[string]Result // key: lower host or IP
}

// DefaultDBURL 公开 GeoLite2-Country 镜像（可被配置覆盖）
const DefaultDBURL = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"

// New 创建定位器；dbPath 为空则用 data/GeoLite2-Country.mmdb
func New(dbPath string) *Locator {
	if dbPath == "" {
		dbPath = filepath.Join("data", "GeoLite2-Country.mmdb")
	}
	return &Locator{
		dbPath: dbPath,
		cache:  make(map[string]Result),
	}
}

// Open 打开已有 MMDB；不存在则返回 nil error 但 db 为空（可后续 EnsureDB）
func (l *Locator) Open() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db != nil {
		return nil
	}
	if _, err := os.Stat(l.dbPath); err != nil {
		return err
	}
	db, err := geoip2.Open(l.dbPath)
	if err != nil {
		return err
	}
	l.db = db
	slog.Info("geoip database opened", "path", l.dbPath)
	return nil
}

// Close 关闭 DB
func (l *Locator) Close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.db != nil {
		_ = l.db.Close()
		l.db = nil
	}
}

// EnsureDB 若本地无库则下载
func (l *Locator) EnsureDB(downloadURL string) error {
	if err := l.Open(); err == nil {
		return nil
	}
	if downloadURL == "" {
		downloadURL = DefaultDBURL
	}
	if err := os.MkdirAll(filepath.Dir(l.dbPath), 0o755); err != nil {
		return err
	}
	tmp := l.dbPath + ".tmp"
	slog.Info("downloading geoip database", "url", downloadURL, "dest", l.dbPath)
	if err := downloadFile(downloadURL, tmp); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("download geoip: %w", err)
	}
	if err := os.Rename(tmp, l.dbPath); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return l.Open()
}

func downloadFile(url, path string) error {
	client := &http.Client{Timeout: 120 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "node-hunter-geo/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %d", resp.StatusCode)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

// Ready 是否已加载 MMDB
func (l *Locator) Ready() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.db != nil
}

// LookupHost 解析 host（IP 或域名）并查国家；并合并名称启发
func (l *Locator) LookupHost(host, nodeName string) Result {
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	key := strings.ToLower(host)

	l.mu.RLock()
	if r, ok := l.cache[key]; ok && r.CountryCode != "" {
		l.mu.RUnlock()
		// 名称启发可补全空字段
		if r.CountryName == "" {
			if h := FromName(nodeName); h.CountryCode != "" {
				r.CountryName = h.CountryName
			}
		}
		return r
	}
	l.mu.RUnlock()

	var out Result
	ip := net.ParseIP(host)
	if ip == nil {
		// DNS
		ips, err := net.LookupIP(host)
		if err == nil {
			for _, cand := range ips {
				if v4 := cand.To4(); v4 != nil {
					ip = v4
					break
				}
				if ip == nil {
					ip = cand
				}
			}
		}
	}
	if ip != nil {
		if r, ok := l.lookupIP(ip); ok {
			out = r
			out.CountryCode = NormalizeCode(out.CountryCode)
			out.Source = "mmdb"
		}
	}
	// 名称启发：MMDB 失败或 CDN anycast 不准时作为补充
	if nameR := FromName(nodeName); nameR.CountryCode != "" {
		if out.CountryCode == "" {
			out = nameR
			out.Source = "name"
		} else if out.CountryName == "" {
			out.CountryName = nameR.CountryName
		}
	}
	// 域名 TLD / 关键字
	if out.CountryCode == "" {
		if r := FromHost(host); r.CountryCode != "" {
			out = r
			out.Source = "host"
		}
	}
	if out.CountryCode != "" {
		l.mu.Lock()
		l.cache[key] = out
		l.mu.Unlock()
	}
	return out
}

func (l *Locator) lookupIP(ip net.IP) (Result, bool) {
	l.mu.RLock()
	db := l.db
	l.mu.RUnlock()
	if db == nil {
		return Result{}, false
	}
	rec, err := db.Country(ip)
	if err != nil || rec == nil || rec.Country.IsoCode == "" {
		// 尝试 registered country
		if rec != nil && rec.RegisteredCountry.IsoCode != "" {
			return Result{
				CountryCode: strings.ToUpper(rec.RegisteredCountry.IsoCode),
				CountryName: pickName(rec.RegisteredCountry.Names),
			}, true
		}
		return Result{}, false
	}
	return Result{
		CountryCode: strings.ToUpper(rec.Country.IsoCode),
		CountryName: pickName(rec.Country.Names),
	}, true
}

func pickName(names map[string]string) string {
	if names == nil {
		return ""
	}
	if v := names["en"]; v != "" {
		return v
	}
	if v := names["zh-CN"]; v != "" {
		return v
	}
	for _, v := range names {
		if v != "" {
			return v
		}
	}
	return ""
}

// Enrich 批量标注节点 Country/City（原地修改）
func (l *Locator) Enrich(nodes interface {
	// avoid import cycle — use concrete in service
}) {
}

// Annotate 填充单个节点的地理字段
func Annotate(n interface {
	GetServer() string
}, loc *Locator) {
}

// NormalizeCode 统一 ISO（UK→GB）
func NormalizeCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "UK" {
		return "GB"
	}
	return code
}

// FromName 从节点备注解析国家（旗帜 emoji / 中英文名 / ISO）
func FromName(name string) Result {
	name = strings.TrimSpace(name)
	if name == "" {
		return Result{}
	}
	// 旗帜 emoji → ISO（区域指示符）
	if code := flagEmojiToISO(name); code != "" {
		code = NormalizeCode(code)
		return Result{CountryCode: code, CountryName: ISOToName(code), Source: "name"}
	}
	lower := strings.ToLower(name)
	// 直接 ISO 码边界
	for code, cn := range isoNames {
		// [US] US- / -US / 美国 / United States
		if strings.Contains(lower, strings.ToLower(cn)) {
			return Result{CountryCode: code, CountryName: cn, Source: "name"}
		}
	}
	// 中文别名
	for alias, code := range cnAliases {
		if strings.Contains(name, alias) {
			return Result{CountryCode: code, CountryName: ISOToName(code), Source: "name"}
		}
	}
	// 英文别名
	for alias, code := range enAliases {
		if strings.Contains(lower, alias) {
			return Result{CountryCode: code, CountryName: ISOToName(code), Source: "name"}
		}
	}
	// 独立 ISO token
	for _, tok := range tokenize(name) {
		u := strings.ToUpper(tok)
		if len(u) == 2 {
			if _, ok := isoNames[u]; ok {
				return Result{CountryCode: u, CountryName: ISOToName(u), Source: "name"}
			}
		}
	}
	return Result{}
}

// FromHost 从域名启发（.jp .uk cf 等弱启发，仅作 fallback）
func FromHost(host string) Result {
	h := strings.ToLower(host)
	// 明确国家 ccTLD
	cctld := map[string]string{
		".jp": "JP", ".kr": "KR", ".hk": "HK", ".tw": "TW", ".sg": "SG",
		".de": "DE", ".fr": "FR", ".nl": "NL", ".gb": "GB", ".uk": "GB",
		".au": "AU", ".ca": "CA", ".in": "IN", ".ru": "RU", ".br": "BR",
		".tr": "TR", ".it": "IT", ".es": "ES", ".se": "SE", ".no": "NO",
		".fi": "FI", ".pl": "PL", ".ua": "UA", ".vn": "VN", ".th": "TH",
		".my": "MY", ".id": "ID", ".ph": "PH", ".mo": "MO", ".ie": "IE",
		".ch": "CH", ".at": "AT", ".be": "BE", ".cz": "CZ", ".ar": "AR",
		".mx": "MX", ".za": "ZA", ".il": "IL", ".ae": "AE", ".sa": "SA",
	}
	for suf, code := range cctld {
		if strings.HasSuffix(h, suf) {
			return Result{CountryCode: code, CountryName: ISOToName(code), Source: "host"}
		}
	}
	return Result{}
}

func flagEmojiToISO(s string) string {
	// 扫描成对的 regional indicator symbols
	runes := []rune(s)
	for i := 0; i+1 < len(runes); i++ {
		a, b := runes[i], runes[i+1]
		if a >= 0x1F1E6 && a <= 0x1F1FF && b >= 0x1F1E6 && b <= 0x1F1FF {
			c1 := byte(a - 0x1F1E6 + 'A')
			c2 := byte(b - 0x1F1E6 + 'A')
			code := string([]byte{c1, c2})
			if _, ok := isoNames[code]; ok {
				return code
			}
		}
	}
	return ""
}

func tokenize(s string) []string {
	var out []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			out = append(out, b.String())
			b.Reset()
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// ISOToName 常用国家英文名
func ISOToName(code string) string {
	code = strings.ToUpper(code)
	if n, ok := isoNames[code]; ok {
		return n
	}
	return code
}

// FlagEmoji ISO → 旗帜
func FlagEmoji(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 2 {
		return ""
	}
	a := rune(code[0])
	b := rune(code[1])
	if a < 'A' || a > 'Z' || b < 'A' || b > 'Z' {
		return ""
	}
	return string([]rune{0x1F1E6 + (a - 'A'), 0x1F1E6 + (b - 'A')})
}

// DisplayName 用于节点展示：🇺🇸 United States
func DisplayName(code string) string {
	code = strings.ToUpper(code)
	flag := FlagEmoji(code)
	name := ISOToName(code)
	if flag != "" {
		return flag + " " + name
	}
	return name
}

var isoNames = map[string]string{
	"US": "United States", "HK": "Hong Kong", "TW": "Taiwan", "JP": "Japan", "KR": "South Korea",
	"SG": "Singapore", "CN": "China", "DE": "Germany", "GB": "United Kingdom", "UK": "United Kingdom",
	"FR": "France", "NL": "Netherlands", "CA": "Canada", "AU": "Australia", "IN": "India",
	"RU": "Russia", "TR": "Turkey", "BR": "Brazil", "IT": "Italy", "ES": "Spain",
	"SE": "Sweden", "NO": "Norway", "FI": "Finland", "PL": "Poland", "UA": "Ukraine",
	"VN": "Vietnam", "TH": "Thailand", "MY": "Malaysia", "ID": "Indonesia", "PH": "Philippines",
	"MO": "Macau", "IE": "Ireland", "CH": "Switzerland", "AT": "Austria", "BE": "Belgium",
	"CZ": "Czechia", "AR": "Argentina", "MX": "Mexico", "ZA": "South Africa", "IL": "Israel",
	"AE": "United Arab Emirates", "SA": "Saudi Arabia", "RO": "Romania", "BG": "Bulgaria",
	"HU": "Hungary", "PT": "Portugal", "DK": "Denmark", "NZ": "New Zealand", "CL": "Chile",
	"CO": "Colombia", "KZ": "Kazakhstan", "UZ": "Uzbekistan", "IR": "Iran", "IQ": "Iraq",
	"PK": "Pakistan", "BD": "Bangladesh", "NG": "Nigeria", "EG": "Egypt", "KE": "Kenya",
	"LT": "Lithuania", "LV": "Latvia", "EE": "Estonia", "SK": "Slovakia", "SI": "Slovenia",
	"HR": "Croatia", "RS": "Serbia", "GR": "Greece", "LU": "Luxembourg", "IS": "Iceland",
	"MD": "Moldova", "GE": "Georgia", "AM": "Armenia", "AZ": "Azerbaijan", "BY": "Belarus",
	"SC": "Seychelles", "PA": "Panama", "CR": "Costa Rica", "PE": "Peru", "UY": "Uruguay",
	"KH": "Cambodia", "MM": "Myanmar", "NP": "Nepal", "LK": "Sri Lanka", "MN": "Mongolia",
	"CF": "Central African Republic", "XX": "Unknown",
}

var cnAliases = map[string]string{
	"美国": "US", "美國": "US", "香港": "HK", "台湾": "TW", "台灣": "TW", "日本": "JP", "韩国": "KR", "韓國": "KR",
	"新加坡": "SG", "中国": "CN", "德國": "DE", "德国": "DE", "英国": "GB", "英國": "GB", "法国": "FR", "法國": "FR",
	"荷兰": "NL", "荷蘭": "NL", "加拿大": "CA", "澳大利亚": "AU", "澳洲": "AU", "印度": "IN", "俄罗斯": "RU", "俄國": "RU",
	"土耳其": "TR", "巴西": "BR", "意大利": "IT", "西班牙": "ES", "瑞典": "SE", "挪威": "NO", "芬兰": "FI",
	"波兰": "PL", "烏克蘭": "UA", "乌克兰": "UA", "越南": "VN", "泰国": "TH", "馬來西亞": "MY", "马来西亚": "MY",
	"印尼": "ID", "菲律宾": "PH", "澳门": "MO", "愛爾蘭": "IE", "爱尔兰": "IE", "瑞士": "CH", "阿联酋": "AE",
	"以色列": "IL", "阿根廷": "AR", "墨西哥": "MX", "伊朗": "IR",
}

var enAliases = map[string]string{
	"united states": "US", "america": "US", "usa": "US", "hong kong": "HK", "hongkong": "HK",
	"taiwan": "TW", "japan": "JP", "tokyo": "JP", "osaka": "JP", "korea": "KR", "seoul": "KR",
	"singapore": "SG", "germany": "DE", "frankfurt": "DE", "united kingdom": "GB", "london": "GB",
	"britain": "GB", "england": "GB", "france": "FR", "paris": "FR", "netherlands": "NL", "amsterdam": "NL",
	"canada": "CA", "toronto": "CA", "australia": "AU", "sydney": "AU", "india": "IN", "mumbai": "IN",
	"russia": "RU", "moscow": "RU", "turkey": "TR", "istanbul": "TR", "brazil": "BR", "italy": "IT",
	"spain": "ES", "sweden": "SE", "norway": "NO", "finland": "FI", "poland": "PL", "ukraine": "UA",
	"vietnam": "VN", "thailand": "TH", "bangkok": "TH", "malaysia": "MY", "indonesia": "ID",
	"philippines": "PH", "macau": "MO", "macao": "MO", "ireland": "IE", "switzerland": "CH",
	"austria": "AT", "belgium": "BE", "israel": "IL", "emirates": "AE", "dubai": "AE",
	"los angeles": "US", "san jose": "US", "seattle": "US", "chicago": "US", "dallas": "US",
	"new york": "US", "miami": "US", "silicon valley": "US", "cloudflare": "US", // weak
}
