package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36"

func imageHost(host string) bool {
	return strings.HasSuffix(host, ".hdslb.com") || strings.HasSuffix(host, ".biliimg.com")
}
func bilibiliAvatarURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || !imageHost(u.Hostname()) || u.User != nil || u.Port() != "" {
		return ""
	}
	if u.Scheme == "http" || u.Scheme == "" {
		u.Scheme = "https"
	}
	if u.Scheme != "https" {
		return ""
	}
	return u.String()
}
func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) > 5 {
		return errors.New("重定向过多")
	}
	if req.URL.Scheme != "https" || (!imageHost(req.URL.Hostname()) && req.URL.Hostname() != "api.bilibili.com") {
		return errors.New("拒绝非 B 站图片服务的重定向")
	}
	return nil
}

type dynamicModule struct {
	Desc *struct {
		Text string `json:"text"`
	} `json:"desc"`
	Major *struct {
		Archive *struct {
			BVID     string `json:"bvid"`
			Title    string `json:"title"`
			Cover    string `json:"cover"`
			Duration string `json:"duration_text"`
		} `json:"archive"`
		Draw *struct {
			Items []struct {
				Src string `json:"src"`
			} `json:"items"`
		} `json:"draw"`
		Opus *struct {
			Title   string `json:"title"`
			Summary struct {
				Text string `json:"text"`
			} `json:"summary"`
			Pics []struct {
				URL string `json:"url"`
			} `json:"pics"`
		} `json:"opus"`
	} `json:"major"`
}
type dynamicItem struct {
	ID      string `json:"id_str"`
	Type    string `json:"type"`
	Modules struct {
		Author struct {
			Name string `json:"name"`
			Face string `json:"face"`
		} `json:"module_author"`
		Dynamic dynamicModule `json:"module_dynamic"`
	} `json:"modules"`
	Orig *dynamicItem `json:"orig"`
}

// Web uses an object; the desktop API uses an ordered module array.
func (item *dynamicItem) UnmarshalJSON(data []byte) error {
	var raw struct {
		ID      string          `json:"id_str"`
		Type    string          `json:"type"`
		Modules json.RawMessage `json:"modules"`
		Orig    *dynamicItem    `json:"orig"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	item.ID = raw.ID
	item.Type = raw.Type
	item.Orig = raw.Orig
	if !bytes.HasPrefix(bytes.TrimSpace(raw.Modules), []byte("[")) {
		if len(raw.Modules) == 0 {
			return nil
		}
		return json.Unmarshal(raw.Modules, &item.Modules)
	}
	var modules []struct {
		Author *struct {
			Name string `json:"name"`
			Face string `json:"face"`
			User struct {
				Name string `json:"name"`
				Face string `json:"face"`
			} `json:"user"`
		} `json:"module_author"`
		Desc    json.RawMessage `json:"module_desc"`
		Dynamic *struct {
			Archive json.RawMessage `json:"dyn_archive"`
			Draw    json.RawMessage `json:"dyn_draw"`
			Opus    json.RawMessage `json:"dyn_opus"`
			Forward *struct {
				Item *dynamicItem `json:"item"`
			} `json:"dyn_forward"`
		} `json:"module_dynamic"`
	}
	if err := json.Unmarshal(raw.Modules, &modules); err != nil {
		return err
	}
	for _, m := range modules {
		if m.Author != nil {
			item.Modules.Author.Name = m.Author.Name
			item.Modules.Author.Face = m.Author.Face
			if m.Author.User.Name != "" {
				item.Modules.Author.Name = m.Author.User.Name
			}
			if m.Author.User.Face != "" {
				item.Modules.Author.Face = m.Author.User.Face
			}
		}
		if len(m.Desc) > 0 {
			if err := json.Unmarshal(m.Desc, &item.Modules.Dynamic.Desc); err != nil {
				return err
			}
		}
		if m.Dynamic != nil {
			major := map[string]json.RawMessage{}
			if len(m.Dynamic.Archive) > 0 {
				major["archive"] = m.Dynamic.Archive
			}
			if len(m.Dynamic.Draw) > 0 {
				major["draw"] = m.Dynamic.Draw
			}
			if len(m.Dynamic.Opus) > 0 {
				major["opus"] = m.Dynamic.Opus
			}
			if len(major) > 0 {
				data, _ := json.Marshal(major)
				if err := json.Unmarshal(data, &item.Modules.Dynamic.Major); err != nil {
					return err
				}
			}
			if m.Dynamic.Forward != nil {
				item.Orig = m.Dynamic.Forward.Item
			}
		}
	}
	return nil
}

func dynamicContent(item dynamicItem) (string, string, []string) {
	m := item.Modules.Dynamic
	text, title := "", ""
	images := []string{}
	if m.Desc != nil {
		text = m.Desc.Text
	}
	if m.Major != nil {
		if m.Major.Draw != nil {
			for _, pic := range m.Major.Draw.Items {
				images = append(images, pic.Src)
			}
		}
		if m.Major.Opus != nil {
			title = m.Major.Opus.Title
			if text == "" {
				text = m.Major.Opus.Summary.Text
			}
			for _, pic := range m.Major.Opus.Pics {
				images = append(images, pic.URL)
			}
		}
	}
	if item.Orig != nil {
		_, original, pics := dynamicContent(*item.Orig)
		text += "\n\n转发原文：\n" + original
		images = append(images, pics...)
	}
	if title == "" {
		line := strings.Split(text, "\n")[0]
		runes := []rune(line)
		if len(runes) > 44 {
			line = string(runes[:44]) + "…"
		}
		title = line
	}
	return title, text, images
}

type bilibiliFeed struct {
	Videos   []Entry
	Dynamics []Entry
	Name     string
	Avatar   string
	Offset   string
	More     bool
	Complete bool
}

func parseDynamics(r io.Reader) ([]Entry, string, string, bool, error) {
	feed, err := parseBilibiliFeed(r)
	return feed.Dynamics, feed.Name, feed.Offset, feed.More, err
}
func parseBilibiliFeed(r io.Reader) (bilibiliFeed, error) {
	var payload struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    *struct {
			Items  []dynamicItem `json:"items"`
			Offset string        `json:"offset"`
			More   bool          `json:"has_more"`
		} `json:"data"`
	}
	out := bilibiliFeed{Videos: []Entry{}, Dynamics: []Entry{}}
	if err := json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(&payload); err != nil {
		return out, fmt.Errorf("动态响应解析失败: %w", err)
	}
	if payload.Code != 0 {
		return out, fmt.Errorf("B 站动态接口错误 %d: %s；请检查 Cookie 或稍后重试", payload.Code, payload.Message)
	}
	if payload.Data == nil {
		return out, errors.New("动态接口未返回数据，未建立基线")
	}
	out.Offset = payload.Data.Offset
	out.More = payload.Data.More
	for _, item := range payload.Data.Items {
		if item.Modules.Author.Name != "" {
			out.Name = item.Modules.Author.Name
		}
		if avatar := bilibiliAvatarURL(item.Modules.Author.Face); avatar != "" {
			out.Avatar = avatar
		}
		if item.ID == "" {
			continue
		}
		if item.Type == "DYNAMIC_TYPE_AV" {
			major := item.Modules.Dynamic.Major
			if major != nil && major.Archive != nil && major.Archive.BVID != "" {
				video := major.Archive
				var duration float64
				for _, part := range strings.Split(video.Duration, ":") {
					n, _ := strconv.ParseFloat(part, 64)
					duration = duration*60 + n
				}
				cover := video.Cover
				if strings.HasPrefix(cover, "http://") {
					cover = "https://" + strings.TrimPrefix(cover, "http://")
				}
				out.Videos = append(out.Videos, Entry{ID: video.BVID, Title: video.Title, URL: "https://www.bilibili.com/video/" + video.BVID, Thumbnail: cover, Duration: duration})
			}
			continue
		}
		if item.Type != "DYNAMIC_TYPE_DRAW" && item.Type != "DYNAMIC_TYPE_WORD" && item.Type != "DYNAMIC_TYPE_FORWARD" && item.Type != "DYNAMIC_TYPE_ARTICLE" {
			continue
		}
		title, content, images := dynamicContent(item)
		if content == "" && len(images) == 0 {
			continue
		}
		thumbnail := ""
		if len(images) > 0 {
			thumbnail = images[0]
		}
		out.Dynamics = append(out.Dynamics, Entry{ID: item.ID, Title: title, Content: content, Images: images, Thumbnail: thumbnail, URL: "https://t.bilibili.com/" + item.ID})
	}
	return out, nil
}
func cookiesForBilibili(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("Cookie 文件不可读: %w", err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(io.LimitReader(f, 2<<20))
	pairs := []string{}
	for scanner.Scan() {
		line := strings.TrimPrefix(scanner.Text(), "#HttpOnly_")
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != 7 {
			continue
		}
		expires, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil || (expires > 0 && expires <= time.Now().Unix()) {
			continue
		}
		domain := strings.TrimPrefix(parts[0], ".")
		if domain != "bilibili.com" && !strings.HasSuffix(domain, ".bilibili.com") {
			continue
		}
		if strings.ContainsAny(parts[5]+parts[6], "\r\n;") {
			continue
		}
		pairs = append(pairs, parts[5]+"="+parts[6])
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return strings.Join(pairs, "; "), nil
}
func (a *App) fetchBilibiliPage(ctx context.Context, c Creator, settings Settings, offset string) (out bilibiliFeed, resultErr error) {
	defer func() {
		if resultErr != nil && isRiskError(resultErr.Error()) {
			if err := a.transaction(func() error { a.blockBilibiliLocked(resultErr.Error(), time.Now()); return nil }); err != nil {
				resultErr = fmt.Errorf("%w；保存冷却失败: %v", resultErr, err)
			}
		}
	}()
	if a.bilibiliBlocked(time.Now()) {
		return out, errors.New("B 站正在冷却，请等待恢复")
	}
	u, _ := url.Parse(c.URL)
	uid := strings.Split(strings.Trim(u.Path, "/"), "/")[0]
	cookiePath, err := a.bilibiliCookies(ctx, settings)
	if err != nil {
		return out, err
	}
	cookie, err := cookiesForBilibili(cookiePath)
	if err != nil {
		return out, err
	}
	client, err := a.clientFor(settings)
	if err != nil {
		return out, err
	}
	defer client.CloseIdleConnections()
	endpoint := "https://api.bilibili.com/x/polymer/web-dynamic/desktop/v1/feed/space?host_mid=" + uid + "&features=itemOpusStyle&offset=" + url.QueryEscape(offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return out, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", c.URL)
	req.Header.Set("Cookie", cookie)
	resp, err := client.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return out, fmt.Errorf("B 站内容列表 HTTP %d，请检查网络或 Cookie", resp.StatusCode)
	}
	return parseBilibiliFeed(resp.Body)
}

func (a *App) scanBilibili(ctx context.Context, c Creator, settings Settings) (bilibiliFeed, error) {
	out := bilibiliFeed{Videos: []Entry{}, Dynamics: []Entry{}}
	offset := ""
	historyVideos := c.History && c.WatchVideos && !c.Initialized
	historyDynamics := c.History && c.WatchDynamics && !c.DynamicInitialized
	maxPages := 3
	if historyVideos || historyDynamics {
		maxPages = 12
	}
	videoIDs, dynamicIDs := map[string]bool{}, map[string]bool{}
	for page := 0; page < maxPages; page++ {
		feed, err := a.fetchBilibiliPage(ctx, c, settings, offset)
		if err != nil {
			return out, err
		}
		for _, entry := range feed.Videos {
			if !videoIDs[entry.ID] {
				out.Videos = append(out.Videos, entry)
				videoIDs[entry.ID] = true
			}
		}
		for _, entry := range feed.Dynamics {
			if !dynamicIDs[entry.ID] {
				out.Dynamics = append(out.Dynamics, entry)
				dynamicIDs[entry.ID] = true
			}
		}
		if feed.Name != "" {
			out.Name = feed.Name
		}
		if feed.Avatar != "" {
			out.Avatar = feed.Avatar
		}
		if !feed.More {
			out.Complete = true
			break
		}
		if feed.Offset == "" || feed.Offset == offset {
			break
		}
		if historyVideos || historyDynamics {
			if (!historyVideos || len(videoIDs) >= 10) && (!historyDynamics || len(dynamicIDs) >= 10) {
				break
			}
			if page == maxPages-1 {
				return out, fmt.Errorf("历史归档未取全：已检查 %d 页，视频 %d 条、图文 %d 条；未建立首次基线，请稍后重试或改为仅监听后续更新", maxPages, len(videoIDs), len(dynamicIDs))
			}
		}
		offset = feed.Offset
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return out, nil
}
func (a *App) downloadDynamic(ctx context.Context, t Task) (string, []string, int64, error) {
	settings := a.Snapshot().Settings
	client, err := a.clientFor(settings)
	if err != nil {
		return "", nil, 0, err
	}
	defer client.CloseIdleConnections()
	rate := downloadBytesPerSecond(settings)
	paced := &pacedReader{ctx: ctx, rate: rate}
	if rate > 0 {
		client.Timeout = 30*time.Second + time.Duration((30<<20)+1)*time.Second/time.Duration(rate)
	}
	directory := filepath.Join(a.downloadDir, t.CreatorID, t.ID)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return "", nil, 0, err
	}
	files := []string{}
	var total int64
	markdown := "# " + t.Title + "\n\n" + t.Content + "\n\n原文：" + t.URL + "\n"
	for i, raw := range t.Images {
		if i >= 30 {
			return "", nil, 0, errors.New("单条动态图片超过 30 张，已停止归档")
		}
		if strings.HasPrefix(raw, "//") {
			raw = "https:" + raw
		}
		u, err := url.Parse(raw)
		if err == nil && u.Scheme == "http" && imageHost(u.Hostname()) {
			u.Scheme = "https"
			raw = u.String()
		}
		if err != nil || u.Scheme != "https" || !imageHost(u.Hostname()) || u.User != nil || u.Port() != "" {
			return "", nil, 0, errors.New("图片地址不属于受信任的 B 站图片服务")
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return "", nil, 0, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Referer", "https://www.bilibili.com/")
		resp, err := client.Do(req)
		if err != nil {
			return "", nil, 0, err
		}
		contentType := strings.Split(resp.Header.Get("Content-Type"), ";")[0]
		ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif", "image/avif": ".avif"}[contentType]
		if resp.StatusCode != 200 || ext == "" {
			resp.Body.Close()
			return "", nil, 0, fmt.Errorf("图片下载失败 HTTP %d 或图片格式不受支持", resp.StatusCode)
		}
		name := fmt.Sprintf("image-%02d%s", i+1, ext)
		path := filepath.Join(directory, name)
		file, err := os.Create(path + ".part")
		if err != nil {
			resp.Body.Close()
			return "", nil, 0, err
		}
		var source io.Reader = io.LimitReader(resp.Body, (30<<20)+1)
		if rate > 0 {
			paced.source = source
			source = paced
		}
		size, copyErr := io.Copy(file, source)
		resp.Body.Close()
		closeErr := file.Close()
		if copyErr != nil || closeErr != nil || size > 30<<20 {
			os.Remove(path + ".part")
			return "", nil, 0, errors.New("图片下载中断或超过 30MB")
		}
		if err := os.Rename(path+".part", path); err != nil {
			return "", nil, 0, err
		}
		files = append(files, path)
		total += size
		markdown += "\n![图片](" + name + ")\n"
		a.updateProgress(t.ID, float64(i+1)/float64(len(t.Images)+1)*100, "", "", total)
	}
	main := filepath.Join(directory, "content.md")
	if err := os.WriteFile(main, []byte(markdown), 0644); err != nil {
		return "", nil, 0, err
	}
	total += int64(len(markdown))
	metadata, err := json.MarshalIndent(t, "", "  ")
	if err != nil {
		return "", nil, 0, err
	}
	if err = os.WriteFile(filepath.Join(directory, "metadata.json"), metadata, 0644); err != nil {
		return "", nil, 0, err
	}
	return main, files, total, nil
}
