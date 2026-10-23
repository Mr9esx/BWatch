package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type mediaVideo struct {
	Codec       string  `json:"codec"`
	Profile     string  `json:"profile,omitempty"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	FrameRate   float64 `json:"frameRate,omitempty"`
	PixelFormat string  `json:"pixelFormat,omitempty"`
	BitRate     int64   `json:"bitRate,omitempty"`
}

type mediaAudio struct {
	Codec         string `json:"codec"`
	SampleRate    int64  `json:"sampleRate,omitempty"`
	Channels      int    `json:"channels,omitempty"`
	ChannelLayout string `json:"channelLayout,omitempty"`
	BitRate       int64  `json:"bitRate,omitempty"`
}

type mediaMetadata struct {
	FileName string       `json:"fileName"`
	Format   string       `json:"format"`
	Size     int64        `json:"size"`
	Duration float64      `json:"duration,omitempty"`
	BitRate  int64        `json:"bitRate,omitempty"`
	Video    *mediaVideo  `json:"video,omitempty"`
	Audio    []mediaAudio `json:"audio"`
}

// 参数只在打开文件详情时读取，兼容旧归档，也避免内容库列表批量启动 ffprobe。
func (a *App) serveMediaMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil || index < 0 {
		apiError(w, 400, errors.New("文件索引无效"))
		return
	}
	var path string
	for _, task := range a.Snapshot().Tasks {
		if task.ID != r.PathValue("id") || task.Status != "completed" {
			continue
		}
		if task.Kind == ContentImagePost && index > 0 && index <= len(task.Files) {
			path = task.Files[index-1]
		} else if task.Kind != ContentImagePost && index == 0 {
			path = task.File
		}
		break
	}
	path, err = a.safeFile(path)
	if err != nil || path == "" {
		apiError(w, 404, errors.New("归档媒体文件已移动或不存在"))
		return
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		apiError(w, 503, errors.New("未安装 ffprobe，请安装 FFmpeg 后查看媒体信息"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	info, err := probeMedia(ctx, path)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			apiError(w, 504, errors.New("读取媒体信息超时，请重试"))
		} else {
			apiError(w, 422, errors.New("无法解析媒体信息，文件可能已损坏或格式不受支持"))
		}
		return
	}
	writeJSON(w, 200, info)
}

func probeMedia(ctx context.Context, path string) (mediaMetadata, error) {
	var probe struct {
		Format struct {
			Name     string `json:"format_name"`
			Duration string `json:"duration"`
			BitRate  string `json:"bit_rate"`
		} `json:"format"`
		Streams []struct {
			Type          string `json:"codec_type"`
			Codec         string `json:"codec_name"`
			Profile       string `json:"profile"`
			Width         int    `json:"width"`
			Height        int    `json:"height"`
			FrameRate     string `json:"avg_frame_rate"`
			PixelFormat   string `json:"pix_fmt"`
			BitRate       string `json:"bit_rate"`
			SampleRate    string `json:"sample_rate"`
			Channels      int    `json:"channels"`
			ChannelLayout string `json:"channel_layout"`
			Disposition   struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
	}
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file", "-show_entries", "format=format_name,duration,bit_rate:stream=codec_type,codec_name,profile,width,height,avg_frame_rate,pix_fmt,bit_rate,sample_rate,channels,channel_layout:stream_disposition=attached_pic", "-of", "json", "-i", path)
	out, err := cmd.Output()
	if err != nil {
		return mediaMetadata{}, err
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return mediaMetadata{}, err
	}
	stat, err := os.Stat(path)
	if err != nil {
		return mediaMetadata{}, err
	}
	info := mediaMetadata{FileName: filepath.Base(path), Format: probe.Format.Name, Size: stat.Size(), Duration: mediaNumber(probe.Format.Duration), BitRate: mediaInteger(probe.Format.BitRate), Audio: []mediaAudio{}}
	for _, stream := range probe.Streams {
		switch stream.Type {
		case "video":
			if info.Video == nil && stream.Disposition.AttachedPic == 0 {
				info.Video = &mediaVideo{Codec: stream.Codec, Profile: stream.Profile, Width: stream.Width, Height: stream.Height, FrameRate: mediaNumber(stream.FrameRate), PixelFormat: stream.PixelFormat, BitRate: mediaInteger(stream.BitRate)}
			}
		case "audio":
			info.Audio = append(info.Audio, mediaAudio{Codec: stream.Codec, SampleRate: mediaInteger(stream.SampleRate), Channels: stream.Channels, ChannelLayout: stream.ChannelLayout, BitRate: mediaInteger(stream.BitRate)})
		}
	}
	if info.Video == nil && len(info.Audio) == 0 {
		return mediaMetadata{}, errors.New("文件不包含音视频或图片流")
	}
	return info, nil
}

func mediaNumber(value string) float64 {
	numerator, denominator, ratio := strings.Cut(value, "/")
	n, _ := strconv.ParseFloat(numerator, 64)
	if ratio {
		d, _ := strconv.ParseFloat(denominator, 64)
		n /= d
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n < 0 {
		return 0
	}
	return n
}

func mediaInteger(value string) int64 {
	n, _ := strconv.ParseInt(value, 10, 64)
	if n < 0 {
		return 0
	}
	return n
}
