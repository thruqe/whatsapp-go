package main

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
	"whatsrook/pkg/addons/sdk"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

//go:embed assets/font.ttf
var fontBytes []byte

const (
	width  = 720
	height = 720
	fps    = 30

	moveDuration  = 1.19
	dwellDuration = 0.85
	introDuration = 1.2
	numTicks      = 24
	numParticles  = 32
)

type digitPos struct {
	angle  float64
	radius float64
}

type particle struct {
	angle     float64
	distRatio float64
	size      float64
	speed     float64
	alpha     float64
}

type tick struct {
	angle       float64
	innerOffset float64
	length      float64
	width       float64
	alpha       float64
}

func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4.0 * t * t * t
	}
	p := -2.0*t + 2.0
	return 1.0 - (p * p * p / 2.0)
}

func easeOutBack(t float64) float64 {
	c1 := 1.70158
	c3 := c1 + 1.0
	p := t - 1.0
	return 1.0 + c3*p*p*p + c1*p*p
}

func easeOutCubic(t float64) float64 {
	p := 1.0 - t
	return 1.0 - p*p*p
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func generateDigitPositions(r *rand.Rand) map[rune]digitPos {
	digits := []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
	r.Shuffle(len(digits), func(i, j int) {
		digits[i], digits[j] = digits[j], digits[i]
	})

	startAngle := r.Float64() * 360.0
	cfg := make(map[rune]digitPos, len(digits))
	for i, d := range digits {
		sectorAngle := startAngle + float64(i)*36.0
		angleJitter := (r.Float64() - 0.5) * 4.0
		angle := sectorAngle + angleJitter
		cfg[d] = digitPos{angle: angle, radius: 0.62}
	}
	return cfg
}

func generateParticles(r *rand.Rand) []particle {
	parts := make([]particle, numParticles)
	for i := range parts {
		parts[i] = particle{
			angle:     r.Float64() * math.Pi * 2.0,
			distRatio: 0.2 + r.Float64()*0.95,
			size:      1.5 + r.Float64()*2.8,
			speed:     (r.Float64() - 0.5) * 0.002,
			alpha:     0.4 + r.Float64()*0.6,
		}
	}
	return parts
}

func generateTicks(r *rand.Rand) []tick {
	ticks := make([]tick, numTicks)
	for i := range ticks {
		angle := (float64(i)/float64(numTicks))*math.Pi*2.0 + (r.Float64()-0.5)*0.15
		ticks[i] = tick{
			angle:       angle,
			innerOffset: -12.0 + (r.Float64()-0.5)*16.0,
			length:      14.0 + r.Float64()*22.0,
			width:       1.5 + r.Float64()*1.5,
			alpha:       0.7 + r.Float64()*0.3,
		}
	}
	return ticks
}

func getPos(cfg map[rune]digitPos, digit rune, cx, cy, radius float64) (float64, float64) {
	c, ok := cfg[digit]
	if !ok {
		c = digitPos{angle: 0.0, radius: 0.6}
	}
	rad := c.angle * math.Pi / 180.0
	r := radius * c.radius
	return cx + r*math.Cos(rad), cy + r*math.Sin(rad)
}

// blendPixel applies RGBA premultiplied blending onto img at (x, y)
func blendPixel(img *image.RGBA, x, y int, r, g, b uint8, a float64) {
	if x < 0 || x >= width || y < 0 || y >= height || a <= 0 {
		return
	}
	a = clamp(a, 0.0, 1.0)
	offset := (y*width + x) * 4
	dr := float64(img.Pix[offset])
	dg := float64(img.Pix[offset+1])
	db := float64(img.Pix[offset+2])

	invA := 1.0 - a
	nr := byte(clamp(dr*invA+float64(r)*a, 0, 255))
	ng := byte(clamp(dg*invA+float64(g)*a, 0, 255))
	nb := byte(clamp(db*invA+float64(b)*a, 0, 255))

	img.Pix[offset] = nr
	img.Pix[offset+1] = ng
	img.Pix[offset+2] = nb
	img.Pix[offset+3] = 255
}

func drawFilledCircle(img *image.RGBA, cx, cy, r float64, red, green, blue uint8, alpha float64) {
	if r <= 0 || alpha <= 0 {
		return
	}
	minX := int(math.Floor(cx - r - 1))
	maxX := int(math.Ceil(cx + r + 1))
	minY := int(math.Floor(cy - r - 1))
	maxY := int(math.Ceil(cy + r + 1))

	if minX < 0 {
		minX = 0
	}
	if maxX >= width {
		maxX = width - 1
	}
	if minY < 0 {
		minY = 0
	}
	if maxY >= height {
		maxY = height - 1
	}

	for y := minY; y <= maxY; y++ {
		dy := float64(y) + 0.5 - cy
		dy2 := dy * dy
		for x := minX; x <= maxX; x++ {
			dx := float64(x) + 0.5 - cx
			dist := math.Sqrt(dx*dx + dy2)
			if dist <= r-0.5 {
				blendPixel(img, x, y, red, green, blue, alpha)
			} else if dist < r+0.5 {
				// Anti-aliased edge
				edgeAlpha := alpha * (r + 0.5 - dist)
				blendPixel(img, x, y, red, green, blue, edgeAlpha)
			}
		}
	}
}

func drawStrokedCircle(img *image.RGBA, cx, cy, r, strokeWidth float64, red, green, blue uint8, alpha float64) {
	if r <= 0 || strokeWidth <= 0 || alpha <= 0 {
		return
	}
	halfW := strokeWidth / 2.0
	maxR := r + halfW

	minX := int(math.Floor(cx - maxR - 1))
	maxX := int(math.Ceil(cx + maxR + 1))
	minY := int(math.Floor(cy - maxR - 1))
	maxY := int(math.Ceil(cy + maxR + 1))

	if minX < 0 {
		minX = 0
	}
	if maxX >= width {
		maxX = width - 1
	}
	if minY < 0 {
		minY = 0
	}
	if maxY >= height {
		maxY = height - 1
	}

	for y := minY; y <= maxY; y++ {
		dy := float64(y) + 0.5 - cy
		dy2 := dy * dy
		for x := minX; x <= maxX; x++ {
			dx := float64(x) + 0.5 - cx
			dist := math.Sqrt(dx*dx + dy2)
			delta := math.Abs(dist - r)
			if delta <= halfW-0.5 {
				blendPixel(img, x, y, red, green, blue, alpha)
			} else if delta < halfW+0.5 {
				edgeAlpha := alpha * (halfW + 0.5 - delta)
				blendPixel(img, x, y, red, green, blue, edgeAlpha)
			}
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1, strokeWidth float64, red, green, blue uint8, alpha float64) {
	dx := x1 - x0
	dy := y1 - y0
	dist := math.Hypot(dx, dy)
	if dist <= 0.001 {
		return
	}
	ux := dx / dist
	uy := dy / dist
	halfW := strokeWidth / 2.0

	steps := max(int(dist*2), 1)
	stepDist := dist / float64(steps)

	for i := 0; i <= steps; i++ {
		px := x0 + ux*float64(i)*stepDist
		py := y0 + uy*float64(i)*stepDist
		drawFilledCircle(img, px, py, halfW, red, green, blue, alpha)
	}
}

func drawDashedLine(img *image.RGBA, x0, y0, x1, y1, dashLen, gapLen, strokeWidth float64, red, green, blue uint8, alpha float64) {
	dx := x1 - x0
	dy := y1 - y0
	dist := math.Hypot(dx, dy)
	if dist <= 0.001 {
		return
	}
	ux := dx / dist
	uy := dy / dist

	pos := 0.0
	drawing := true
	for pos < dist {
		segLen := gapLen
		if drawing {
			segLen = dashLen
		}
		end := pos + segLen
		if end > dist {
			end = dist
		}
		if drawing {
			drawLine(img, x0+ux*pos, y0+uy*pos, x0+ux*end, y0+uy*end, strokeWidth, red, green, blue, alpha)
		}
		pos = end
		drawing = !drawing
	}
}

func renderFrame(
	img *image.RGBA,
	parsedFont *opentype.Font,
	sequence []rune,
	cfg map[rune]digitPos,
	particles []particle,
	ticks []tick,
	now float64,
) {
	introRaw := math.Min(now/introDuration, 1.0)
	introEase := easeOutCubic(introRaw)
	introScale := easeOutBack(math.Min(introRaw*1.1, 1.0))

	floatY := math.Sin(now*1.5) * 7.0 * introEase
	floatX := math.Cos(now*1.1) * 4.0 * introEase

	cx := float64(width)/2.0 + floatX
	cy := float64(height)/2.0 + floatY
	baseDialRadius := math.Max(math.Min(float64(width), float64(height))*0.38, 1.0)
	dialRadius := baseDialRadius * math.Max(introScale, 0.01)

	// Clear background: #0c0f17 (12, 15, 23)
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i] = 12
		img.Pix[i+1] = 15
		img.Pix[i+2] = 23
		img.Pix[i+3] = 255
	}

	openingMoveStart := introDuration
	openingMoveEnd := introDuration + moveDuration

	var currentX, currentY float64
	if len(sequence) == 0 || now < openingMoveStart {
		currentX, currentY = cx, cy
	} else if now < openingMoveEnd {
		p := easeInOutCubic((now - openingMoveStart) / moveDuration)
		toX, toY := getPos(cfg, sequence[0], cx, cy, dialRadius)
		currentX = cx + (toX-cx)*p
		currentY = cy + (toY-cy)*p
	} else {
		cycleLen := moveDuration + dwellDuration
		elapsed := now - openingMoveEnd
		cycleIdx := int(elapsed/cycleLen) % len(sequence)
		nextIdx := (cycleIdx + 1) % len(sequence)
		phaseT := math.Mod(elapsed, cycleLen)

		fromX, fromY := getPos(cfg, sequence[cycleIdx], cx, cy, dialRadius)
		toX, toY := getPos(cfg, sequence[nextIdx], cx, cy, dialRadius)

		if phaseT < moveDuration {
			p := easeInOutCubic(phaseT / moveDuration)
			currentX = fromX + (toX-fromX)*p
			currentY = fromY + (toY-fromY)*p
		} else {
			currentX, currentY = toX, toY
		}
	}

	// 1. Draw main dial circle (Fill #161618, Stroke #ffffff)
	drawFilledCircle(img, cx, cy, dialRadius, 22, 22, 24, 1.0)
	strokeW := math.Max(dialRadius*0.015, 3.0)
	strokeA := math.Min(introEase*1.2, 1.0)
	drawStrokedCircle(img, cx, cy, dialRadius, strokeW, 255, 255, 255, strokeA)

	// 2. Draw Ticks
	for _, t := range ticks {
		cos := math.Cos(t.angle)
		sin := math.Sin(t.angle)
		rInner := dialRadius + t.innerOffset
		rOuter := rInner + t.length*introEase
		drawLine(img, cx+rInner*cos, cy+rInner*sin, cx+rOuter*cos, cy+rOuter*sin, t.width, 255, 255, 255, clamp(t.alpha*introEase, 0, 1))
	}

	// 3. Draw Particles
	for _, p := range particles {
		angle := p.angle + p.speed*now*1000.0
		pr := dialRadius * p.distRatio
		px := cx + pr*math.Cos(angle)
		py := cy + pr*math.Sin(angle)
		pSize := math.Max(p.size*introEase, 0.1)
		drawFilledCircle(img, px, py, pSize, 255, 255, 255, clamp(p.alpha*introEase, 0, 1))
	}

	// 4. Draw Dashed Line to Indicator Badge
	drawDashedLine(img, cx, cy, currentX, currentY, 4.0, 6.0, 1.5, 255, 255, 255, 0.08*introEase)

	// 5. Draw Badge Outer Glow and Main
	badgeRadius := dialRadius * 0.14
	drawFilledCircle(img, currentX, currentY, math.Max((badgeRadius+2.0)*introEase, 1.0), 43, 91, 102, clamp(0.4*introEase, 0, 1))
	drawFilledCircle(img, currentX, currentY, math.Max(badgeRadius*introEase, 1.0), 43, 91, 102, clamp(introEase, 0, 1))
	drawStrokedCircle(img, currentX, currentY, math.Max(badgeRadius*introEase, 1.0), 2.2, 255, 255, 255, clamp(introEase, 0, 1))

	// 6. Draw Digits using opentype font face
	fontSize := baseDialRadius * 0.38
	face, err := opentype.NewFace(parsedFont, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err == nil {
		defer face.Close()
		textDrawer := &font.Drawer{
			Dst:  img,
			Src:  image.NewUniform(color.RGBA{R: 255, G: 255, B: 255, A: uint8(clamp(introEase*255.0, 0, 255))}),
			Face: face,
		}

		for digit := range cfg {
			x, y := getPos(cfg, digit, cx, cy, dialRadius)
			s := string(digit)
			bounds, _ := textDrawer.BoundString(s)
			strW := float64(bounds.Max.X-bounds.Min.X) / 64.0
			strH := float64(bounds.Max.Y-bounds.Min.Y) / 64.0

			dotX := fixed.I(int(math.Round(x - strW/2.0)))
			dotY := fixed.I(int(math.Round(y + strH/2.0)))
			textDrawer.Dot = fixed.Point26_6{X: dotX, Y: dotY}
			textDrawer.DrawString(s)
		}
	}
}

func generateVideo(code string, seconds float64, seed *int64) ([]byte, error) {
	if code == "" {
		return nil, fmt.Errorf("captcha code must not be empty")
	}
	for _, c := range code {
		if !unicode.IsDigit(c) {
			return nil, fmt.Errorf("captcha code must be numeric, got %q", code)
		}
	}

	parsedFont, err := opentype.Parse(fontBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse embedded font: %w", err)
	}

	sequence := []rune(code)
	var r *rand.Rand
	if seed != nil {
		r = rand.New(rand.NewSource(*seed))
	} else {
		r = rand.New(rand.NewSource(rand.Int63()))
	}

	cfg := generateDigitPositions(r)
	particles := generateParticles(r)
	ticks := generateTicks(r)

	duration := seconds
	if duration <= 0.0 {
		duration = 8.0
	}
	totalFrames := int(math.Round(duration * float64(fps)))

	tmpDir := os.TempDir()
	tmpPath := filepath.Join(tmpDir, fmt.Sprintf("captcha_%d_%d.mp4", os.Getpid(), rand.Uint32()))

	ffmpegCmd := exec.Command("ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", width, height),
		"-framerate", fmt.Sprintf("%d", fps),
		"-i", "-",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-crf", "23",
		"-movflags", "+faststart",
		tmpPath,
	)

	stdin, err := ffmpegCmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to open ffmpeg stdin: %w", err)
	}

	if err := ffmpegCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, width, height))

	for i := range totalFrames {
		t := float64(i) / float64(fps)
		renderFrame(img, parsedFont, sequence, cfg, particles, ticks, t)
		if _, err := stdin.Write(img.Pix); err != nil {
			_ = ffmpegCmd.Process.Kill()
			_ = os.Remove(tmpPath)
			return nil, fmt.Errorf("failed writing frame %d to ffmpeg: %w", i, err)
		}
	}
	stdin.Close()

	if err := ffmpegCmd.Wait(); err != nil {
		_ = os.Remove(tmpPath)
		return nil, fmt.Errorf("ffmpeg execution failed: %w", err)
	}

	videoBytes, err := os.ReadFile(tmpPath)
	_ = os.Remove(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read video output: %w", err)
	}

	return videoBytes, nil
}

func main() {
	req := sdk.Load()
	query := req.Query()

	// Check CLI invocation: captcha [CODE] [OUTPUT_FILE]
	if len(os.Args) > 1 && !strings.HasPrefix(os.Args[1], "{") {
		firstArg := strings.TrimSpace(os.Args[1])
		if firstArg == "--help" || firstArg == "-h" {
			fmt.Println("Usage: captcha [CODE] [OUTPUT_FILE]")
			fmt.Println("Example: captcha 3681 out.mp4")
			return
		}

		code := firstArg
		isNum := len(code) > 0
		for _, c := range code {
			if !unicode.IsDigit(c) {
				isNum = false
				break
			}
		}
		if !isNum {
			code = fmt.Sprintf("%04d", rand.Intn(10000))
		}

		outputFile := "captcha.mp4"
		if len(os.Args) > 2 {
			outputFile = os.Args[2]
		}

		fmt.Printf("Generating captcha video for code %s -> %s...\n", code, outputFile)
		bytes, err := generateVideo(code, 8.0, nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Video generation failed: %s\n", err)
			os.Exit(1)
		}

		if err := os.WriteFile(outputFile, bytes, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving video to %s: %s\n", outputFile, err)
			os.Exit(1)
		}
		fmt.Printf("Saved %d bytes to %s\n", len(bytes), outputFile)
		return
	}

	// WhatsRook Plugin Protocol Handler
	if query == "" || strings.EqualFold(query, "help") {
		sdk.Respond(fmt.Sprintf(
			"*%s Captcha Generator*\n\n"+
				"• `%scaptcha <code>` : Generate an animated verification code video (e.g. `%scaptcha 3681`)\n"+
				"• `%scaptcha test`   : Generate a random 4-digit verification video demo",
			req.EffectiveBotName(),
			req.EffectivePrefix(),
			req.EffectivePrefix(),
			req.EffectivePrefix(),
		))
		return
	}

	var code string
	if strings.EqualFold(query, "test") || strings.EqualFold(query, "demo") {
		code = fmt.Sprintf("%04d", rand.Intn(10000))
	} else {
		clean := strings.TrimSpace(query)
		isNum := len(clean) >= 3 && len(clean) <= 8
		for _, c := range clean {
			if !unicode.IsDigit(c) {
				isNum = false
				break
			}
		}
		if !isNum {
			sdk.RespondErr("Invalid captcha code. Please provide a numeric code between 3 and 8 digits (e.g. `3681`).")
			return
		}
		code = clean
	}

	bytes, err := generateVideo(code, 8.0, nil)
	if err != nil {
		sdk.RespondErr(fmt.Sprintf("Failed to generate captcha video: %s", err))
		return
	}

	dataURL := sdk.ToDataURL("video/mp4", sdk.EncodeBase64(bytes))
	caption := fmt.Sprintf("🎬 *Verification Captcha*\nCode: `%s`", code)
	sdk.SendVideoFull(dataURL, caption, "video/mp4", true)
}
