package mediainfo

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

const MaxOutputBytes = 16 << 20

type Metadata struct {
	Media    map[string]string
	Video    []map[string]string
	Audio    []map[string]string
	Text     []map[string]string
	Chapters map[string]string
	Image    map[string]string
	Menu     map[string]string
	Bindings map[string]string
}

func Decode(input io.Reader) (Metadata, error) {
	data, err := io.ReadAll(io.LimitReader(input, MaxOutputBytes+1))
	if err != nil {
		return Metadata{}, fmt.Errorf("read MediaInfo output: %w", err)
	}
	return DecodeBytes(data)
}

func DecodeString(input string) (Metadata, error) {
	return DecodeBytes([]byte(input))
}

func DecodeBytes(data []byte) (Metadata, error) {
	if len(data) > MaxOutputBytes {
		return Metadata{}, fmt.Errorf("MediaInfo output exceeds %d bytes", MaxOutputBytes)
	}
	var document struct {
		Media struct {
			Tracks []map[string]any `json:"track"`
		} `json:"media"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return Metadata{}, fmt.Errorf("decode MediaInfo JSON: %w", err)
	}
	result := Metadata{Bindings: make(map[string]string)}
	for _, raw := range document.Media.Tracks {
		kind, _ := raw["@type"].(string)
		values := stringMap(raw)
		delete(values, "@type")
		switch kind {
		case "General":
			if result.Media == nil {
				result.Media = values
			}
		case "Video":
			result.Video = append(result.Video, values)
		case "Audio":
			result.Audio = append(result.Audio, values)
		case "Text":
			result.Text = append(result.Text, values)
		case "Image":
			if result.Image == nil {
				result.Image = values
			}
		case "Menu":
			if result.Menu == nil {
				result.Menu = values
				result.Chapters = chapters(values)
			}
		}
	}
	if result.Media == nil {
		return Metadata{}, fmt.Errorf("MediaInfo JSON contains no General track")
	}
	result.Bindings = bindings(result)
	return result, nil
}

func stringMap(values map[string]any) map[string]string {
	result := make(map[string]string, len(values))
	for key, value := range values {
		if key == "extra" {
			if extra, ok := value.(map[string]any); ok {
				for extraKey, extraValue := range stringMap(extra) {
					result[extraKey] = extraValue
				}
			}
			continue
		}
		switch current := value.(type) {
		case string:
			result[key] = current
		case json.Number:
			result[key] = current.String()
		case float64:
			result[key] = strconv.FormatFloat(current, 'f', -1, 64)
		case bool:
			result[key] = strconv.FormatBool(current)
		}
	}
	return result
}

func chapters(menu map[string]string) map[string]string {
	result := make(map[string]string)
	for key, value := range menu {
		if len(key) >= 9 && key[2] == '_' && key[5] == '_' {
			result[key] = value
		}
	}
	return result
}

func bindings(metadata Metadata) map[string]string {
	result := make(map[string]string)
	video := first(metadata.Video)
	audio := first(metadata.Audio)
	set := func(name string, values ...string) {
		for _, value := range values {
			if value != "" {
				result[name] = value
				return
			}
		}
	}

	set("cf", container(metadata.Media["Format"]))
	set("vcf", video["Format"])
	set("vc", codec(video["Encoded_Library_Name"], video["Encoded_Library"], video["Format"]))
	set("ac", audioCodec(audio))
	set("aco", audioCodecProfile(audio))
	set("width", digits(video["Width"]))
	set("height", digits(video["Height"]))
	if result["width"] != "" && result["height"] != "" {
		result["resolution"] = result["width"] + "x" + result["height"]
		result["vf"] = result["height"] + scan(video)
	}
	if height, _ := strconv.Atoi(result["height"]); height >= 2160 {
		result["vk"] = "4K"
	}
	set("hpi", result["height"]+scan(video))
	set("bitdepth", digits(video["BitDepth"]))
	set("hdr", hdr(video))
	if strings.Contains(strings.ToLower(video["HDR_Format"]), "dolby vision") {
		result["dovi"] = "Dolby Vision"
	}
	set("bitrate", bitRate(metadata.Media["OverallBitRate"]))
	set("vbr", bitRate(video["BitRate"]))
	set("abr", bitRate(audio["BitRate"]))
	set("fps", frameRate(video["FrameRate"]))
	set("khz", samplingRate(audio["SamplingRate"]))
	set("ar", aspectRatio(video["DisplayAspectRatio"]))
	set("af", channelCount(audio["Channels"]))
	set("channels", channelLayout(audio["Channels"]))
	set("acf", audioChannelFormat(audio))
	set("hd", definition(result["height"]))
	if ratio, err := strconv.ParseFloat(video["DisplayAspectRatio"], 64); err == nil && ratio > 1.37 {
		result["ws"] = "WS"
	}
	set("s3d", video["MultiView_Layout"])
	set("mediaTitle", metadata.Media["Title"])
	set("audioLanguages", languages(metadata.Audio))
	set("textLanguages", languages(metadata.Text))

	if milliseconds, err := strconv.ParseFloat(metadata.Media["Duration"], 64); err == nil {
		duration := time.Duration(milliseconds * float64(time.Millisecond))
		seconds := int64(math.Round(duration.Seconds()))
		result["duration"] = duration.String()
		result["seconds"] = strconv.FormatInt(seconds, 10)
		result["minutes"] = strconv.FormatInt(seconds/60, 10)
		result["hours"] = fmt.Sprintf("%d:%02d", seconds/3600, seconds%3600/60)
	}
	return result
}

func first(values []map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	return values[0]
}

func digits(value string) string {
	var result strings.Builder
	for _, character := range value {
		if character >= '0' && character <= '9' {
			result.WriteRune(character)
		}
	}
	return result.String()
}

func container(value string) string {
	switch strings.ToLower(value) {
	case "matroska":
		return "mkv"
	case "mpeg-4":
		return "mp4"
	}
	return strings.ToLower(value)
}

func codec(values ...string) string {
	for _, value := range values {
		lower := strings.ToLower(value)
		switch {
		case strings.Contains(lower, "x265"):
			return "x265"
		case strings.Contains(lower, "x264"):
			return "x264"
		case strings.Contains(lower, "hevc"):
			return "HEVC"
		case strings.Contains(lower, "avc"):
			return "AVC"
		case value != "":
			return value
		}
	}
	return ""
}

func audioCodec(audio map[string]string) string {
	value := strings.ToLower(audio["Format"] + " " + audio["Format_Commercial_IfAny"])
	switch {
	case strings.Contains(value, "truehd") || strings.Contains(value, "mlp fba"):
		return "truehd"
	case strings.Contains(value, "e-ac-3"):
		return "eac3"
	case strings.Contains(value, "ac-3"):
		return "ac3"
	case strings.Contains(value, "dts"):
		return "dts"
	case strings.Contains(value, "aac"):
		return "aac"
	}
	return strings.ToLower(audio["Format"])
}

func audioCodecProfile(audio map[string]string) string {
	value := strings.ToLower(audio["Format_Commercial_IfAny"] + " " + audio["Format_Profile"])
	switch {
	case strings.Contains(value, "truehd") && strings.Contains(value, "atmos"):
		return "TrueHD+Atmos"
	case strings.Contains(value, "atmos"):
		return audioCodec(audio) + "+Atmos"
	default:
		return audio["Format_Commercial_IfAny"]
	}
}

func hdr(video map[string]string) string {
	value := strings.ToLower(video["HDR_Format"])
	compatibility := strings.ToLower(video["HDR_Format_Compatibility"])
	switch {
	case strings.Contains(value, "dolby vision") && strings.Contains(compatibility, "hdr10"):
		return "DV+HDR10"
	case strings.Contains(value, "dolby vision"):
		return "DV"
	case strings.Contains(value, "hdr10+"):
		return "HDR10+"
	case strings.Contains(value, "hlg"):
		return "HLG"
	case strings.Contains(compatibility, "hdr10") || strings.Contains(value, "smpte st 2086"):
		return "HDR10"
	}
	return ""
}

func scan(video map[string]string) string {
	if strings.EqualFold(video["ScanType"], "Interlaced") {
		return "i"
	}
	return "p"
}

func bitRate(value string) string {
	rate, err := strconv.ParseFloat(digits(value), 64)
	if err != nil || rate == 0 {
		return ""
	}
	if rate >= 1_000_000 {
		return strconv.FormatFloat(rate/1_000_000, 'f', 1, 64) + " Mbps"
	}
	return strconv.FormatFloat(rate/1000, 'f', 0, 64) + " kbps"
}

func frameRate(value string) string {
	if value == "" {
		return ""
	}
	return value + " fps"
}

func samplingRate(value string) string {
	rate, err := strconv.ParseFloat(digits(value), 64)
	if err != nil || rate == 0 {
		return ""
	}
	return strconv.FormatFloat(rate/1000, 'f', -1, 64) + " kHz"
}

func aspectRatio(value string) string {
	ratio, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return value
	}
	if math.Abs(ratio-1.778) < .02 {
		return "16∶9"
	}
	return value
}

func channelCount(value string) string {
	if count := digits(value); count != "" {
		return count + "ch"
	}
	return ""
}

func channelLayout(value string) string {
	switch digits(value) {
	case "8":
		return "7.1"
	case "6":
		return "5.1"
	case "2":
		return "2.0"
	case "1":
		return "1.0"
	}
	return ""
}

func audioChannelFormat(audio map[string]string) string {
	channels := channelLayout(audio["Channels"])
	codec := strings.ToUpper(audioCodec(audio))
	if codec == "TRUEHD" {
		codec = "TrueHD"
	}
	if codec == "" || channels == "" {
		return ""
	}
	return codec + channels
}

func definition(height string) string {
	value, _ := strconv.Atoi(height)
	switch {
	case value >= 2160:
		return "UHD"
	case value >= 720:
		return "HD"
	case value > 0:
		return "SD"
	default:
		return ""
	}
}

func languages(streams []map[string]string) string {
	var values []string
	seen := make(map[string]bool)
	for _, stream := range streams {
		value := stream["Language"]
		if value != "" && !seen[value] {
			seen[value] = true
			values = append(values, value)
		}
	}
	return strings.Join(values, ",")
}
