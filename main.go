package main

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strings"
	"time"

	"honnef.co/go/curve"
	"honnef.co/go/jello"
	"honnef.co/go/jello/engine/wgpu_engine"
	"honnef.co/go/jello/gfx"
	"honnef.co/go/jello/mem"
	"honnef.co/go/jello/renderer"
	wayland "honnef.co/go/libwayland"
	"honnef.co/go/wgpu"
)

func main() {
	// runtime.MemProfileRate = 1
	go func() {
		http.ListenAndServe("localhost:8080", nil)
	}()

	dsp, err := wayland.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer dsp.Disconnect()

	reg := dsp.Registry()
	defer reg.Destroy()

	var comp *wayland.Compositor
	var shm *wayland.Shm
	var xdg *wayland.XdgWmBase
	var dcm *wayland.XdgDecorationManager
	reg.OnGlobal = func(name uint32, iface string, version uint32) {
		switch iface {
		case "wl_compositor":
			comp = reg.BindCompositor(name, 4)
		case "wl_shm":
			shm = reg.BindShm(name, 1)
		case "xdg_wm_base":
			xdg = reg.BindXdgWmBase(name, 2)
			xdg.OnPing = func(serial uint32) {
				xdg.Pong(serial)
			}
		case "zxdg_decoration_manager_v1":
			dcm = reg.BindZxdgDecorationManagerV1(name, 1)
		}
	}

	// Roundtrip calls dsp.Sync and waits for the callback to fire, so at this point we
	// know that we've seen all of the globals that existed when we created the registry.
	dsp.Roundtrip()

	if comp == nil {
		log.Fatal("Wayland server has no compositor global")
	}
	if shm == nil {
		log.Fatal("Wayland server has no shm global")
	}
	if xdg == nil {
		log.Fatal("Wayland server has no xdg_wm_base global")
	}
	if dcm == nil {
		// XXX in a real application we'd also want to support the KDE one, and fall back to client side decorations
		// otherwise.
		// log.Fatal("Wayland server has no zxdg_decoration_manager_v1 global")
	}

	surf := comp.CreateSurface()
	defer surf.Destroy()

	xdgSurf := xdg.XdgSurface(surf)
	defer xdgSurf.Destroy()

	xdgSurf.OnConfigure = func(serial uint32) {
		xdgSurf.AckConfigure(serial)
	}

	top := xdgSurf.Toplevel()
	defer top.Destroy()

	top.SetTitle("Foobar")
	if dcm != nil {
		dec := dcm.ToplevelDecoration(top)
		dec.SetMode(wayland.XdgToplevelDecorationModeServerSide)
	}
	surf.Commit()

	wgpu.SetLogLevel(wgpu.LogLevelError)
	ins := wgpu.CreateInstance(wgpu.InstanceDescriptor{
		Extras: &wgpu.InstanceExtras{
			Backends: wgpu.InstanceBackendVulkan,
		},
	})
	defer ins.Release()

	surface := ins.CreateSurface(wgpu.SurfaceDescriptor{
		Label: "our surface",
		Native: wgpu.WaylandSurface{
			Display: dsp.Handle(),
			Surface: surf.Handle(),
		},
	})
	defer surface.Release()

	adapter, err := ins.RequestAdapter(wgpu.RequestAdapterOptions{
		PowerPreference:      wgpu.PowerPreferenceHighPerformance,
		ForceFallbackAdapter: false,
		// CompatibleSurface:    surface,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer adapter.Release()

	limits := wgpu.DefaultLimits
	limits.MaxSampledTexturesPerShaderStage = 8388606
	dev, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredFeatures: []wgpu.FeatureName{
			wgpu.FeatureNameTimestampQuery,
			wgpu.NativeFeatureNameTextureBindingArray,
			wgpu.NativeFeatureNameSampledTextureAndStorageBufferArrayNonUniformIndexing,
			wgpu.NativeFeatureNamePartiallyBoundBindingArray,
		},
		RequiredLimits: &wgpu.RequiredLimits{
			Limits: limits,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer dev.Release()

	surface.Configure(dev, &wgpu.SurfaceConfiguration{
		Width:       800,
		Height:      800,
		Format:      wgpu.TextureFormatRGBA8UnormSrgb,
		Usage:       wgpu.TextureUsageRenderAttachment,
		PresentMode: wgpu.PresentModeMailbox,
		AlphaMode:   wgpu.CompositeAlphaModeAuto,
	})

	queue := dev.Queue()
	defer queue.Release()

	r := wgpu_engine.New(dev, &wgpu_engine.RendererOptions{
		SurfaceFormat: wgpu.TextureFormatRGBA8UnormSrgb,
		UseCPU:        false,
	})

	scene := &jello.Scene{}

	{
		type rect struct {
			r curve.Rect
			c gfx.Color
		}

		// Horizontal and vertical stripes, demonstrating gamma-corrected alpha
		// blending.
		rects := []rect{
			{
				curve.Rect{20, 20, 60, 500},
				gfx.SRGB{24, 234, 34, 255},
			},
			{
				curve.Rect{70, 20, 110, 500},
				gfx.SRGB{234, 232, 24, 255},
			},
			{
				curve.Rect{120, 20, 160, 500},
				gfx.SRGB{24, 234, 200, 255},
			},
			{
				curve.Rect{170, 20, 210, 500},
				gfx.SRGB{238, 70, 166, 255},
			},

			{
				curve.Rect{0, 40, 800, 50},
				gfx.SRGB{255, 0, 0, 255},
			},
			{
				curve.Rect{0, 50, 800, 60},
				gfx.SRGB{255, 0, 0, 128},
			},
			{
				curve.Rect{0, 100, 800, 110},
				gfx.SRGB{0, 127, 255, 255},
			},
			{
				curve.Rect{0, 110, 800, 120},
				gfx.SRGB{0, 127, 255, 128},
			},
			{
				curve.Rect{0, 160, 800, 170},
				gfx.SRGB{147, 255, 0, 255},
			},
			{
				curve.Rect{0, 170, 800, 180},
				gfx.SRGB{147, 255, 0, 128},
			},
		}
		for _, r := range rects {
			scene.Fill(
				gfx.NonZero,
				curve.Identity,
				gfx.SolidBrush{
					Color: r.c,
				},
				curve.Identity,
				r.r.PathElements(0.1),
			)
		}
	}

	{
		// Slightly askew color gradient in sRGB, gamma-corrected, also
		// demonstrating concatenating scenes.
		subscene := &jello.Scene{}
		subscene.Fill(
			gfx.NonZero,
			curve.Identity,
			gfx.GradientBrush{
				Gradient: gfx.LinearGradient{
					Start: curve.Pt(20, 0),
					End:   curve.Pt(700, 0),
					Stops: []gfx.ColorStop{
						{
							Offset: 0,
							Color:  gfx.LinearSRGB{1, 1, 0, 1},
						},
						{
							Offset: 1,
							Color:  gfx.LinearSRGB{0, 1, 0, 1},
						},
					},
				},
			},
			curve.Identity,
			curve.Rect{20, 500, 700, 550}.PathElements(0.1),
		)
		scene.Append(subscene, curve.Rotate(-0.1))
	}

	{
		// Color gradient in Oklch.
		scene.Fill(
			gfx.NonZero,
			curve.Identity,
			gfx.GradientBrush{
				Gradient: gfx.LinearGradient{
					Start: curve.Pt(20, 0),
					End:   curve.Pt(700, 0),
					Stops: []gfx.ColorStop{
						{
							Offset: 0,
							Color:  gfx.Oklch{0.628, 0.25768330773615683, 29.2338851923426, 1},
						},
						{
							Offset: 1,
							Color:  gfx.Oklch{0.8664396115356693, 0.2948272403370167, 142.49533888780996, 1},
						},
					},
				},
			},
			curve.Identity,
			curve.Rect{20, 550, 700, 600}.PathElements(0.1),
		)
	}

	{
		// 8x8 grid of mes, rotated about _some_ point
		f, err := os.Open("./img.png")
		if err != nil {
			panic(err)
		}
		img, err := png.Decode(f)
		if err != nil {
			panic(err)
		}
		f.Close()
		for i := 0; i < 8; i++ {
			for j := 0; j < 8; j++ {
				scene.Fill(
					gfx.NonZero,
					curve.Scale(0.02, 0.02).ThenRotateAbout(0.1*float64(i*j), curve.Pt(5, 5)).ThenTranslate(curve.Vec(float64(i)*20+620, float64(j)*20+620)),
					gfx.ImageBrush{
						Image: gfx.Image{
							Image: img,
						},
					},
					curve.Identity,
					curve.Rect{0, 0, float64(img.Bounds().Dx()), float64(img.Bounds().Dy())}.PathElements(0.1),
				)
			}
		}
	}

	{
		// Another me, clipped by a circle, translucent.
		f, err := os.Open("./img2.png")
		if err != nil {
			panic(err)
		}
		img, err := png.Decode(f)
		if err != nil {
			panic(err)
		}
		f.Close()
		var subscene jello.Scene
		subscene.Fill(
			gfx.NonZero,
			curve.Identity,
			gfx.ImageBrush{
				Image: gfx.Image{
					Image: img,
				},
			},
			curve.Identity,
			curve.Rect{0, 0, float64(img.Bounds().Dx()), float64(img.Bounds().Dy())}.PathElements(0.1),
		)
		scene.PushLayer(gfx.BlendMode{}, 0.9, curve.Identity, curve.Circle{Center: curve.Pt(400, 300), Radius: 100}.PathElements(0.1))
		scene.Append(&subscene, curve.Identity)
		scene.PopLayer()
	}

	{
		// f(x) = (x⁴ + x³ - 13x² - x) / 10
		p := curve.FitToBezPathOpt(Quartic{}, 0.00005)
		scene.Stroke(
			curve.DefaultStroke.
				WithCaps(curve.RoundCap).WithWidth(1),
			curve.Scale(10, 10).ThenTranslate(curve.Vec(400, 700)),
			gfx.SolidBrush{Color: gfx.LinearSRGB{1, 0, 0, 1}},
			curve.Identity,
			p.Elements(),
		)
	}

	tex := dev.CreateTexture(&wgpu.TextureDescriptor{
		Label: "target texture",
		Size: wgpu.Extent3D{
			Width:              800,
			Height:             800,
			DepthOrArrayLayers: 1,
		},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Usage:         wgpu.TextureUsageStorageBinding | wgpu.TextureUsageTextureBinding,
		Format:        wgpu.TextureFormatRGBA8Unorm,
	})
	defer tex.Release()
	view := tex.CreateView(nil)
	defer view.Release()

	var frame uint64

	const debug = true
	var prof *wgpu_engine.Profiler
	if debug {
		prof = wgpu_engine.NewProfiler(dev)
	}

	arena := mem.NewArena()
	vararg := make([]*wgpu.CommandBuffer, 1)
	for {
		// if frame == 32000 {
		// 	f, _ := os.Create("mem.pprof")
		// 	runtime.GC()
		// 	pprof.WriteHeapProfile(f)
		// 	return
		// }
		arena.Reset()
		results := prof.Collect()
		if debug {
			for i := range results {
				// Buffer output to avoid flicker in terminal
				// OPT(dh): reuse buffer
				var buf bytes.Buffer
				printProfilerResult(&buf, &results[i], 0)
				fmt.Fprintln(&buf)
				fmt.Println(buf.String())
			}
		}

		pgroup := prof.Start(frame)
		frame++

		span := pgroup.Nest("wayland dispatch")
		dsp.Dispatch()
		span.End()

		func() {
			span := pgroup.Nest("CurrentTexture")
			surfaceTexture, err := surface.CurrentTexture()
			if err != nil {
				panic(err)
			}
			span.End()
			defer surfaceTexture.Texture.Release()

			r.RenderToSurface(
				arena,
				queue,
				scene.Encoding(),
				&surfaceTexture,
				&renderer.RenderParams{
					BaseColor:          gfx.LinearSRGB{1, 1, 1, 1},
					Width:              800,
					Height:             800,
					AntialiasingMethod: renderer.Area,
				},
				pgroup,
			)

			pgroup.End()
			enc := dev.CreateCommandEncoder(nil)
			defer enc.Release()
			prof.Resolve(enc)
			cmd := enc.Finish(nil)
			defer cmd.Release()
			vararg[0] = cmd
			dev.Queue().Submit(vararg...)
			prof.Map()

			surface.Present()
			dev.Poll(false)
		}()
	}
}

func printProfilerResult(w io.Writer, res *wgpu_engine.ProfilerResult, depth int) {
	nesting := strings.Repeat("  ", depth+1)
	qnesting := strings.Repeat("  ", depth+2)

	if depth == 0 {
		fmt.Fprintln(w, "Frame", res.Tag)
	} else {
		fmt.Fprintf(w, "%sGroup %s\n", strings.Repeat("  ", depth), res.Label)
	}
	fmt.Fprintf(w, "%sCPU time: %s\n", nesting, formatDuration(res.CPUEnd.Sub(res.CPUStart)))
	if len(res.Queries) != 0 {
		fmt.Fprintf(w, "%sGPU Queries:\n", nesting)
		for _, q := range res.Queries {
			fmt.Fprintf(w, "%s%s: %s\n", qnesting, q.Label, formatDuration(time.Duration(q.End-q.Start)))
		}
	}
	if len(res.Children) != 0 {
		for i := range res.Children {
			printProfilerResult(w, &res.Children[i], depth+1)
		}
	}
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.3f µs", float64(d.Nanoseconds())/1000)
}

type Quartic struct{}

var _ curve.FittableCurve = Quartic{}

// eval evaluates f(x) = (x⁴ + x³ - 13x² - x) / 10
func eval(x float64) float64 {
	return (x*x*x*x + x*x*x - 13*x*x - x) / 10
}

// evalDeriv evaluates the derivative of [eval], f'(x) = (4x³ + 3x² - 26x - 1) / 10
func evalDeriv(x float64) float64 {
	return (4*x*x*x + 3*x*x - 26*x - 1) / 10
}

// BreakCusp implements curve.ParamCurveFit.
func (q Quartic) BreakCusp(start float64, end float64) (float64, bool) {
	return 0, false
}

// SamplePtDeriv implements curve.ParamCurveFit.
func (q Quartic) SamplePtDeriv(t float64) (curve.Point, curve.Vec2) {
	// t ∈ [0, 1] but we want to plot our function from x=-4.5 to x=3.5
	x := (1-t)*-4.5 + t*3.5
	// We negate y and dy because our coordinate system is y-down
	y := -eval(x)
	dx := 1.0
	dy := -evalDeriv(x)
	return curve.Pt(x, y), curve.Vec(dx, dy)
}

// SamplePtTangent implements curve.ParamCurveFit.
func (q Quartic) SamplePtTangent(t float64, sign float64) curve.CurveFitSample {
	p, tangent := q.SamplePtDeriv(t)
	return curve.CurveFitSample{
		Point:   p,
		Tangent: tangent,
	}
}
