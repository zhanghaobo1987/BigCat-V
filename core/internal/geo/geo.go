// Package geo 管理 GeoIP / GeoSite 规则数据库的下载与定时更新。
//
//   - sing-box 使用本地 .srs 二进制 rule_set（本包负责下载到 geoDir）；
//   - xray 使用 geoip.dat / geosite.dat（同样由本包下载，引擎通过
//     XRAY_LOCATION_ASSET 环境变量指向 geoDir）。
package geo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Asset 待下载的资源。
type Asset struct {
	Name string // 本地文件名，如 geoip-cn.srs
	URL  string // 下载地址
}

// DefaultAssets 默认资源集（sing-box .srs + xray .dat）。
func DefaultAssets() []Asset {
	return []Asset{
		{"geoip-cn.srs", "https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip-cn.srs"},
		{"geosite-cn.srs", "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-cn.srs"},
		{"geosite-geolocation-!cn.srs", "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-geolocation-!cn.srs"},
		{"geosite-category-ads-all.srs", "https://github.com/SagerNet/sing-geosite/releases/latest/download/geosite-category-ads-all.srs"},
		{"geoip.dat", "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geoip.dat"},
		{"geosite.dat", "https://github.com/Loyalsoldier/v2ray-rules-dat/releases/latest/download/geosite.dat"},
	}
}

// FileMeta 单个文件的元信息。
type FileMeta struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	Exists    bool      `json:"exists"`
}

// Settings 更新设置（持久化在 geoDir/meta.json）。
type Settings struct {
	AutoUpdate    bool                 `json:"auto_update"`
	IntervalHours int                  `json:"interval_hours"`
	Files         map[string]*FileMeta `json:"files"`
}

const maxAssetSize = 100 << 20 // 100MB 上限

// Updater 资源更新器。
type Updater struct {
	mu      sync.Mutex // 保护 set（短临界区，不跨网络 I/O）
	upMu    sync.Mutex // 串行化 Update 调用
	dir     string
	assets  []Asset
	set     Settings
	client  *http.Client
	onEvent func(msg string) // 状态事件回调（可为 nil）
}

// New 创建更新器（dir 为 workdir/geo，不存在则创建）。
func New(dir string, onEvent func(string)) (*Updater, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("创建 geo 目录失败: %w", err)
	}
	u := &Updater{
		dir:     dir,
		assets:  DefaultAssets(),
		client:  &http.Client{Timeout: 120 * time.Second},
		onEvent: onEvent,
		set: Settings{
			AutoUpdate:    true,
			IntervalHours: 24,
			Files:         map[string]*FileMeta{},
		},
	}
	if data, err := os.ReadFile(u.metaPath()); err == nil {
		var s Settings
		if json.Unmarshal(data, &s) == nil {
			if s.IntervalHours <= 0 {
				s.IntervalHours = 24
			}
			if s.Files == nil {
				s.Files = map[string]*FileMeta{}
			}
			u.set = s
		}
	}
	// 补齐新版本新增的资源条目
	for _, a := range u.assets {
		if _, ok := u.set.Files[a.Name]; !ok {
			u.set.Files[a.Name] = &FileMeta{Name: a.Name, URL: a.URL}
		}
	}
	u.refreshExists()
	_ = u.saveSettings()
	return u, nil
}

func (u *Updater) metaPath() string { return filepath.Join(u.dir, "meta.json") }

// Dir 资源目录。
func (u *Updater) Dir() string { return u.dir }

// Path 资源本地路径。
func (u *Updater) Path(name string) string { return filepath.Join(u.dir, name) }

// Has 文件是否存在。
func (u *Updater) Has(name string) bool {
	return u.set.Files[name] != nil && u.set.Files[name].Exists
}

func (u *Updater) refreshExists() {
	for _, a := range u.assets {
		m := u.set.Files[a.Name]
		m.URL = a.URL
		if fi, err := os.Stat(u.Path(a.Name)); err == nil {
			m.Exists = true
			m.Size = fi.Size()
			if m.UpdatedAt.IsZero() {
				m.UpdatedAt = fi.ModTime()
			}
		} else {
			m.Exists = false
			m.Size = 0
		}
	}
}

func (u *Updater) saveSettings() error {
	data, err := json.MarshalIndent(u.set, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(u.metaPath(), data, 0600)
}

// Status 返回当前状态（副本）。
func (u *Updater) Status() Settings {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.refreshExists()
	out := u.set
	out.Files = make(map[string]*FileMeta, len(u.set.Files))
	for k, v := range u.set.Files {
		cp := *v
		out.Files[k] = &cp
	}
	return out
}

// SetAutoUpdate 设置自动更新开关与间隔。
func (u *Updater) SetAutoUpdate(on bool, hours int) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.set.AutoUpdate = on
	if hours >= 1 && hours <= 24*30 {
		u.set.IntervalHours = hours
	}
	return u.saveSettings()
}

func (u *Updater) emit(msg string) {
	if u.onEvent != nil {
		u.onEvent(msg)
	}
}

// Update 下载缺失或全部资源。onlyMissing=true 时只补缺失文件。
// 网络下载期间不持有状态锁，Status/SetAutoUpdate 不会被阻塞。
func (u *Updater) Update(ctx context.Context, onlyMissing bool) error {
	u.upMu.Lock()
	defer u.upMu.Unlock()

	// 快照任务列表（短临界区）
	type job struct {
		a    Asset
		skip bool
	}
	u.mu.Lock()
	jobs := make([]job, 0, len(u.assets))
	for _, a := range u.assets {
		jobs = append(jobs, job{a: a, skip: onlyMissing && u.set.Files[a.Name].Exists})
	}
	u.mu.Unlock()

	var firstErr error
	for _, j := range jobs {
		if j.skip {
			continue
		}
		u.emit("正在下载 " + j.a.Name)
		if err := u.download(ctx, j.a); err != nil {
			u.emit("下载失败 " + j.a.Name + ": " + err.Error())
			if firstErr == nil {
				firstErr = fmt.Errorf("%s: %w", j.a.Name, err)
			}
			continue
		}
		u.emit("已更新 " + j.a.Name)
	}
	u.mu.Lock()
	u.refreshExists()
	_ = u.saveSettings()
	u.mu.Unlock()
	return firstErr
}

func (u *Updater) download(ctx context.Context, a Asset) error {
	req, err := http.NewRequestWithContext(ctx, "GET", a.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "BigCatV/geo-updater")
	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	tmp := u.Path(a.Name) + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	n, err := io.CopyN(f, resp.Body, maxAssetSize+1)
	f.Close()
	if err != nil && err != io.EOF {
		os.Remove(tmp)
		return err
	}
	if n > maxAssetSize {
		os.Remove(tmp)
		return fmt.Errorf("文件过大（>%dMB）", maxAssetSize>>20)
	}
	if n == 0 {
		os.Remove(tmp)
		return fmt.Errorf("空文件")
	}
	if err := os.Rename(tmp, u.Path(a.Name)); err != nil {
		return err
	}
	// 仅元数据更新需要锁
	u.mu.Lock()
	m := u.set.Files[a.Name]
	m.Exists = true
	m.Size = n
	m.UpdatedAt = time.Now()
	u.mu.Unlock()
	return nil
}

// Start 启动定时更新协程：启动时补齐缺失文件，之后按间隔全量更新。
// ctx 取消时退出。
func (u *Updater) Start(ctx context.Context) {
	go func() {
		// 启动时先补齐缺失
		_ = u.Update(ctx, true)
		for {
			u.mu.Lock()
			on := u.set.AutoUpdate
			hours := u.set.IntervalHours
			u.mu.Unlock()
			if hours <= 0 {
				hours = 24
			}
			timer := time.NewTimer(time.Duration(hours) * time.Hour)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
				if on {
					_ = u.Update(ctx, false)
				}
			}
		}
	}()
}
