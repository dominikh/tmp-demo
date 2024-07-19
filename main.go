package main ////

import (
	"bytes"
	"fmt"
	"image/png"
	"io"
	"log"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime"
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
	runtime.MemProfileRate = 1
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

	// stroke := curve.DefaultStroke.WithWidth(6)

	type rect struct {
		r curve.Rect
		c gfx.Color
	}

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

	scene := &jello.Scene{}

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

	scene2 := &jello.Scene{}

	scene2.Fill(
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

	scene.Append(scene2, curve.Rotate(-0.1))

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

	if true {
		f, err := os.Open("./img.png")
		if err != nil {
			panic(err)
		}
		img1, err := png.Decode(f)
		if err != nil {
			panic(err)
		}
		f.Close()

		f, err = os.Open("./img2.png")
		if err != nil {
			panic(err)
		}
		img2, err := png.Decode(f)
		if err != nil {
			panic(err)
		}
		f.Close()
		// img2 = img2.(*image.NRGBA).SubImage(image.Rect(300, 300, 500, 500))

		f, err = os.Open("./img3.png")
		if err != nil {
			panic(err)
		}
		img3, err := png.Decode(f)
		if err != nil {
			panic(err)
		}
		f.Close()

		for i := 0; i < 16*3; i++ {
			for j := 0; j < 16*3; j++ {
				scene.Fill(
					gfx.NonZero,
					curve.Scale(0.02, 0.02).ThenRotate(0.01*float64(i*j)).ThenTranslate(curve.Vec(float64(i)*20, float64(j)*20)),
					gfx.ImageBrush{
						Image: gfx.Image{
							Image: img1,
						},
					},
					curve.Identity,
					curve.Rect{0, 0, float64(img1.Bounds().Dx()), float64(img1.Bounds().Dy())}.PathElements(0.1),
				)
			}
		}

		if true {
			scene.Fill(
				gfx.NonZero,
				curve.Scale(0.1, 0.1),
				gfx.ImageBrush{
					Image: gfx.Image{
						Image: img3,
					},
				},
				curve.Identity,
				curve.Rect{0, 0, float64(img1.Bounds().Dx()), float64(img1.Bounds().Dy())}.PathElements(0.1),
			)

			var scene3 jello.Scene
			scene3.Fill(
				gfx.NonZero,
				curve.Identity,
				gfx.ImageBrush{
					Image: gfx.Image{
						Image: img2,
					},
				},
				curve.Identity,
				curve.Rect{0, 0, float64(img2.Bounds().Dx()), float64(img2.Bounds().Dy())}.PathElements(0.1),
			)
			scene.Append(&scene3, curve.Identity)
		}
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

	var deg int
	var frame uint64

	const debug = false
	var prof *wgpu_engine.Profiler
	if debug {
		prof = wgpu_engine.NewProfiler(dev)
	}

	var report wgpu.GlobalReport
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

		if debug && false {
			// OPT(dh): reuse buffer
			var buf bytes.Buffer
			ins.Report(&report)
			printReport(&buf, &report)
			fmt.Fprintln(&buf)
			fmt.Println(buf.String())
		}

		pgroup := prof.Start(frame)
		frame++

		deg = (deg + 1) % 360
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

func printReport(w io.Writer, report *wgpu.GlobalReport) {
	printRegistryReport(w, &report.Surfaces, "surfaces.")

	switch report.BackendType {
	case wgpu.BackendTypeD3D12:
		printHubReport(w, &report.DX12, "\tdx12.")
	case wgpu.BackendTypeMetal:
		printHubReport(w, &report.Metal, "\tmetal.")
	case wgpu.BackendTypeVulkan:
		printHubReport(w, &report.Vulkan, "\tvulkan.")
	case wgpu.BackendTypeOpenGL:
		printHubReport(w, &report.GL, "\tgl.")
	}
}

func printRegistryReport(w io.Writer, report *wgpu.RegistryReport, prefix string) {
	fmt.Fprintf(w, "%snumAllocated=%d\n", prefix, report.NumAllocated)
	fmt.Fprintf(w, "%snumKeptFromUser=%d\n", prefix, report.NumKeptFromUser)
	fmt.Fprintf(w, "%snumReleasedFromUser=%d\n", prefix, report.NumReleasedFromUser)
	fmt.Fprintf(w, "%snumError=%d\n", prefix, report.NumError)
	fmt.Fprintf(w, "%selementSize=%d\n", prefix, report.ElementSize)
}

func printHubReport(w io.Writer, report *wgpu.HubReport, prefix string) {
	printRegistryReport(w, &report.Adapters, prefix+"adapter.")
	printRegistryReport(w, &report.Devices, prefix+"devices.")
	printRegistryReport(w, &report.Queues, prefix+"queues.")
	printRegistryReport(w, &report.PipelineLayouts, prefix+"pipelineLayouts.")
	printRegistryReport(w, &report.ShaderModules, prefix+"shaderModules.")
	printRegistryReport(w, &report.BindGroupLayouts, prefix+"bindGroupLayouts.")
	printRegistryReport(w, &report.BindGroups, prefix+"bindGroups.")
	printRegistryReport(w, &report.CommandBuffers, prefix+"commandBuffers.")
	printRegistryReport(w, &report.RenderBundles, prefix+"renderBundles.")
	printRegistryReport(w, &report.RenderPipelines, prefix+"renderPipelines.")
	printRegistryReport(w, &report.ComputePipelines, prefix+"computePipelines.")
	printRegistryReport(w, &report.QuerySets, prefix+"querySets.")
	printRegistryReport(w, &report.Textures, prefix+"textures.")
	printRegistryReport(w, &report.TextureViews, prefix+"textureViews.")
	printRegistryReport(w, &report.Samplers, prefix+"samplers.")
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
