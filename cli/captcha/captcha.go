// Package captcha generates short verification-code videos: a stylized
// "dial" animation that sweeps through the digits of a code, rendered
// with the gg 2D graphics library and encoded to MP4 via ffmpeg.
//
// Requires ffmpeg to be installed and available on PATH. No other
// external files are needed — the font used for rendering digits is
// embedded directly in the compiled binary.
//
// Basic usage:
//
//	f, err := os.Create("out.mp4")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer f.Close()
//
//	if err := captcha.Generate(f, "3681", captcha.Options{}); err != nil {
//	    log.Fatal(err)
//	}
package captcha

import (
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strconv"

	"github.com/fogleman/gg"
	"github.com/golang/freetype/truetype"
	"golang.org/x/image/font"
)

//go:embed assets/font.ttf
var fontBytes []byte

var parsedFont *truetype.Font

func init() {
	var err error
	parsedFont, err = truetype.Parse(fontBytes)
	if err != nil {
		panic(fmt.Sprintf("captcha: failed to parse embedded font: %v", err))
	}
}

const (
	Width, Height = 720, 720
	FPS           = 30

	moveDuration  = 1.19
	dwellDuration = 0.85
	introDuration = 1.2
	numTicks      = 24
	numParticles  = 32
)

type digitPos struct {
	angle, radius float64
}

type particle struct {
	angle, distRatio, size, speed, alpha float64
}

type tick struct {
	angle, innerOffset, length, width, alpha float64
}

// Options configures video generation. The zero value is usable and
// falls back to sensible defaults.
type Options struct {
	// Seed controls the random layout of digits around the dial and the
	// particle field. Same seed + same code always produces the same
	// video. Leave zero to let Generate pick a random seed.
	Seed int64
	// Seconds is the total video duration. Defaults to 8 if zero or negative.
	Seconds float64
}

// Generate renders a verification-code video for the given numeric code
// and writes the resulting MP4 to w. It requires ffmpeg to be installed
// and available on PATH.
func Generate(w io.Writer, code string, opts Options) error {
	if code == "" {
		return fmt.Errorf("captcha: code must not be empty")
	}
	for _, c := range code {
		if c < '0' || c > '9' {
			return fmt.Errorf("captcha: code must be numeric, got %q", code)
		}
	}

	seconds := opts.Seconds
	if seconds <= 0 {
		seconds = 8.0
	}
	seed := opts.Seed
	if seed == 0 {
		seed = rand.Int63()
	}

	digits := make([]string, len(code))
	for i, r := range code {
		digits[i] = string(r)
	}

	rng := rand.New(rand.NewSource(seed))
	cfg := generateDigitPositions(rng)
	particles := generateParticles(rng)
	ticks := generateTicks(rng)

	totalFrames := int(seconds * FPS)

	tmpFile, err := os.CreateTemp("", "captcha-*.mp4")
	if err != nil {
		return fmt.Errorf("captcha: create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()
	defer os.Remove(tmpPath)

	cmd := exec.Command("ffmpeg",
		"-y",
		"-f", "rawvideo",
		"-pix_fmt", "rgba",
		"-s", fmt.Sprintf("%dx%d", Width, Height),
		"-framerate", strconv.Itoa(FPS),
		"-i", "-",
		"-c:v", "libx264",
		"-preset", "ultrafast",
		"-pix_fmt", "yuv420p",
		"-crf", "23",
		"-movflags", "+faststart",
		tmpPath,
	)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("captcha: stdin pipe: %w", err)
	}
	cmd.Stderr = nil

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("captcha: start ffmpeg (is it installed and on PATH?): %w", err)
	}

	fail := func(stage string, err error) error {
		stdin.Close()
		_ = cmd.Process.Kill()
		return fmt.Errorf("captcha: %s: %w", stage, err)
	}

	baseDialRadius := math.Min(Width, Height) * 0.38
	fontSize := baseDialRadius * 0.38
	fontFace := truetype.NewFace(parsedFont, &truetype.Options{Size: fontSize, Hinting: font.HintingFull})

	for i := range totalFrames {
		t := float64(i) / FPS
		dc := gg.NewContext(Width, Height)
		dc.SetFontFace(fontFace)

		renderFrame(dc, digits, cfg, particles, ticks, t)

		img := dc.Image().(*image.RGBA)
		if _, err := stdin.Write(img.Pix); err != nil {
			return fail(fmt.Sprintf("write frame %d to ffmpeg", i), err)
		}
	}
	stdin.Close()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("captcha: ffmpeg failed: %w", err)
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		return fmt.Errorf("captcha: reopen output: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(w, f); err != nil {
		return fmt.Errorf("captcha: copy output: %w", err)
	}
	return nil
}

// --- easing ---

func easeInOutCubic(t float64) float64 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	return 1 - math.Pow(-2*t+2, 3)/2
}

func easeOutBack(t float64) float64 {
	c1 := 1.70158
	c3 := c1 + 1
	return 1 + c3*math.Pow(t-1, 3) + c1*math.Pow(t-1, 2)
}

func easeOutCubic(t float64) float64 {
	return 1 - math.Pow(1-t, 3)
}

func generateDigitPositions(rng *rand.Rand) map[string]digitPos {
	digits := []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}
	rng.Shuffle(len(digits), func(i, j int) { digits[i], digits[j] = digits[j], digits[i] })

	startAngle := rng.Float64() * 360
	cfg := make(map[string]digitPos)
	for i, d := range digits {
		sectorAngle := startAngle + float64(i)*36
		// Much smaller jitter — just enough to feel organic, not enough to
		// break the evenly-spaced ring look.
		angleJitter := (rng.Float64() - 0.5) * 4 // was 14
		angle := sectorAngle + angleJitter

		// Fixed radius, no per-digit variation — keeps all digits on the
		// same clean ring like a real clock face.
		radius := 0.62 // was 0.56 + random 0.10

		cfg[d] = digitPos{angle: angle, radius: radius}
	}
	return cfg
}

func generateParticles(rng *rand.Rand) []particle {
	ps := make([]particle, numParticles)
	for i := range ps {
		ps[i] = particle{
			angle:     rng.Float64() * math.Pi * 2,
			distRatio: 0.2 + rng.Float64()*0.95,
			size:      1.5 + rng.Float64()*2.8,
			speed:     (rng.Float64() - 0.5) * 0.002,
			alpha:     0.4 + rng.Float64()*0.6,
		}
	}
	return ps
}

func generateTicks(rng *rand.Rand) []tick {
	ts := make([]tick, numTicks)
	for i := range ts {
		angle := (float64(i)/numTicks)*math.Pi*2 + (rng.Float64()-0.5)*0.15
		ts[i] = tick{
			angle:       angle,
			innerOffset: -12 + (rng.Float64()-0.5)*16,
			length:      14 + rng.Float64()*22,
			width:       1.5 + rng.Float64()*1.5,
			alpha:       0.7 + rng.Float64()*0.3,
		}
	}
	return ts
}

func getPos(cfg map[string]digitPos, digit string, cx, cy, radius float64) (float64, float64) {
	c, ok := cfg[digit]
	if !ok {
		c = digitPos{angle: 0, radius: 0.6}
	}
	rad := c.angle * math.Pi / 180
	r := radius * c.radius
	return cx + r*math.Cos(rad), cy + r*math.Sin(rad)
}

func renderFrame(dc *gg.Context, sequence []string, cfg map[string]digitPos, particles []particle, ticks []tick, now float64) {
	introRaw := math.Min(now/introDuration, 1.0)
	introEase := easeOutCubic(introRaw)
	introScale := easeOutBack(math.Min(introRaw*1.1, 1.0))

	floatY := math.Sin(now*1.5) * 7 * introEase
	floatX := math.Cos(now*1.1) * 4 * introEase

	cx := Width/2 + floatX
	cy := Height/2 + floatY
	baseDialRadius := math.Min(Width, Height) * 0.38
	dialRadius := baseDialRadius * math.Max(0.01, introScale)

	dc.SetHexColor("#0c0f17")
	dc.Clear()

	var currentX, currentY float64

	// Phase timeline:
	//   [0, introDuration)                     -> badge sits at dial center while dial pops in
	//   [introDuration, introDuration+moveDuration) -> badge travels from center to first digit
	//   [introDuration+moveDuration, ...)       -> normal sequence cycling (2nd digit onward)
	openingMoveStart := introDuration
	openingMoveEnd := introDuration + moveDuration

	switch {
	case len(sequence) == 0:
		currentX, currentY = cx, cy

	case now < openingMoveStart:
		// Badge parked at center during the dial's pop-in animation.
		currentX, currentY = cx, cy

	case now < openingMoveEnd:
		// Traveling from center to the first digit.
		p := easeInOutCubic((now - openingMoveStart) / moveDuration)
		toX, toY := getPos(cfg, sequence[0], cx, cy, dialRadius)
		currentX = cx + (toX-cx)*p
		currentY = cy + (toY-cy)*p

	default:
		// Normal cycling through the rest of the sequence, starting from
		// having just arrived at sequence[0].
		cycleLen := moveDuration + dwellDuration
		elapsedSinceArrival := now - openingMoveEnd
		cycleIdx := int(elapsedSinceArrival/cycleLen) % len(sequence)
		nextIdx := (cycleIdx + 1) % len(sequence)
		phaseT := math.Mod(elapsedSinceArrival, cycleLen)

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

	dc.Push()
	dc.SetRGBA(1, 1, 1, math.Min(1.0, introEase*1.2))
	dc.DrawCircle(cx, cy, dialRadius)
	dc.SetHexColor("#161618")
	dc.FillPreserve()
	dc.SetHexColor("#ffffff")
	dc.SetLineWidth(math.Max(3, dialRadius*0.015))
	dc.Stroke()

	for _, t := range ticks {
		cos, sin := math.Cos(t.angle), math.Sin(t.angle)
		rInner := dialRadius + t.innerOffset
		rOuter := rInner + t.length*introEase
		dc.SetLineCap(gg.LineCapRound)
		dc.SetLineWidth(t.width)
		dc.SetRGBA(1, 1, 1, t.alpha*introEase)
		dc.DrawLine(cx+rInner*cos, cy+rInner*sin, cx+rOuter*cos, cy+rOuter*sin)
		dc.Stroke()
	}

	for _, p := range particles {
		angle := p.angle + p.speed*now*1000
		pr := dialRadius * p.distRatio
		px := cx + pr*math.Cos(angle)
		py := cy + pr*math.Sin(angle)
		dc.SetRGBA(1, 1, 1, p.alpha*introEase)
		dc.DrawCircle(px, py, p.size*introEase)
		dc.Fill()
	}

	dc.SetRGBA(1, 1, 1, 0.08*introEase)
	dc.SetLineWidth(1.5)
	drawDashedLine(dc, cx, cy, currentX, currentY, 4, 6)

	badgeRadius := dialRadius * 0.14
	dc.SetRGBA(43.0/255, 91.0/255, 102.0/255, 0.4)
	dc.DrawCircle(currentX, currentY, math.Max(1, (badgeRadius+2)*introEase))
	dc.Fill()

	dc.SetHexColor("#2b5b66")
	dc.DrawCircle(currentX, currentY, math.Max(1, badgeRadius*introEase))
	dc.FillPreserve()
	dc.SetHexColor("#ffffff")
	dc.SetLineWidth(2.2)
	dc.Stroke()

	for digit := range cfg {
		x, y := getPos(cfg, digit, cx, cy, dialRadius)
		tw, th := dc.MeasureString(digit)
		dc.SetColor(color.White)
		dc.DrawString(digit, x-tw/2, y+th/2-th*0.08)
	}

	dc.Pop()
}

func drawDashedLine(dc *gg.Context, x0, y0, x1, y1, dashLen, gapLen float64) {
	dx, dy := x1-x0, y1-y0
	dist := math.Hypot(dx, dy)
	if dist == 0 {
		return
	}
	ux, uy := dx/dist, dy/dist
	pos := 0.0
	drawing := true
	for pos < dist {
		segLen := dashLen
		if !drawing {
			segLen = gapLen
		}
		end := math.Min(pos+segLen, dist)
		if drawing {
			dc.DrawLine(x0+ux*pos, y0+uy*pos, x0+ux*end, y0+uy*end)
			dc.Stroke()
		}
		pos = end
		drawing = !drawing
	}
}
