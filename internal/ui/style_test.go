package ui

import "testing"

func TestPadOrTrimDisplayPadsAfterTruncatingWideText(t *testing.T) {
	t.Parallel()

	text := "04-27 19:04  扫描  [MOVIE] 复仇者联盟2：奥创纪元 (2015) {tmdb-99861} - 2160p.BluRay.HDR.DV.x265.DDP.7.1-BBR.mkv｜检测到新文件，任务标记为待上传"
	for width := 1; width <= 160; width++ {
		got := padOrTrimDisplay(text, width)
		if gotWidth := displayColumns(got); gotWidth != width {
			t.Fatalf("padOrTrimDisplay() display width = %d, want %d; value=%q", gotWidth, width, got)
		}
	}
}
