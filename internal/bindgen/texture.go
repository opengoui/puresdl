package bindgen

import (
	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// texturePlane describes one pixel buffer of a texture update function: the
// void/uint8 pointer parameter, its pitch parameter and the geometry of the
// plane relative to the updated rectangle.
type texturePlane struct {
	ptr, pitch string // C parameter names
	// half halves the width and height (rounding up), as for the chroma planes
	// of planar YUV. interleaved doubles the width, as for the UV plane of NV12.
	half, interleaved bool
	// scaled multiplies the plane's bytes per sample by the format's bytes per
	// pixel (single-plane RGB formats and the 10-bit NV formats); otherwise a
	// sample is one byte.
	scaled bool
	// formatTail marks a plane that SDL also reads the chroma planes of when
	// the texture format is planar: they follow the rect.H rows of this plane
	// in the same buffer, so the slice must hold them too.
	formatTail bool
}

// textureUpdate describes a texture update function whose buffer sizes derive
// from the texture format and the updated rectangle, which the headers cannot
// express. The []byte form checks every plane before SDL is called, so SDL can
// never read past a slice.
type textureUpdate struct {
	texture, rect string         // C parameter names
	planes        []texturePlane // in parameter order
	formats       []string       // accepted PixelFormat constants; nil accepts any
	name          string         // exported Go name of the []byte form
}

// textureUpdates maps the C texture update functions to their []byte forms.
var textureUpdates = map[string]textureUpdate{
	"SDL_UpdateTexture": {
		texture: "texture", rect: "rect", name: "UpdateTextureBytes",
		planes: []texturePlane{{ptr: "pixels", pitch: "pitch", scaled: true, formatTail: true}},
	},
	"SDL_UpdateYUVTexture": {
		texture: "texture", rect: "rect", name: "UpdateYUVTextureBytes",
		formats: []string{"PIXELFORMAT_YV12", "PIXELFORMAT_IYUV"},
		planes: []texturePlane{
			{ptr: "Yplane", pitch: "Ypitch"},
			{ptr: "Uplane", pitch: "Upitch", half: true},
			{ptr: "Vplane", pitch: "Vpitch", half: true},
		},
	},
	"SDL_UpdateNVTexture": {
		texture: "texture", rect: "rect", name: "UpdateNVTextureBytes",
		formats: []string{"PIXELFORMAT_NV12", "PIXELFORMAT_NV21", "PIXELFORMAT_P010"},
		planes: []texturePlane{
			{ptr: "Yplane", pitch: "Ypitch", scaled: true},
			{ptr: "UVplane", pitch: "UVpitch", half: true, interleaved: true, scaled: true},
		},
	},
}

// textureUpdateOf returns the []byte form of f, or nil when f is not listed or
// its parameters no longer match the table.
func textureUpdateOf(f *ir.Function, plans []argPlan) *textureUpdate {
	tu, ok := textureUpdates[f.Name]
	if !ok || f.Sig.Ret.Resolve().Kind != ir.Bool || len(plans) != 2+2*len(tu.planes) {
		return nil
	}
	want := []string{tu.texture, tu.rect}
	for _, p := range tu.planes {
		want = append(want, p.ptr, p.pitch)
	}
	for i, w := range want {
		if f.Sig.Params[i].Name != w || plans[i].hidden || plans[i].pub == nil {
			return nil
		}
	}
	return &tu
}

// textureUpdateNote is the doc note of the raw wrapper that points at its
// []byte form.
func textureUpdateNote(tu *textureUpdate) string {
	return "The pointers are read for the whole rectangle at the given pitches and are not bounds checked; " + tu.name + " takes byte slices and checks them."
}

// emitTextureUpdate emits the []byte form of a texture update function.
func (g *gen) emitTextureUpdate(f *ir.Function, tu *textureUpdate, plans []argPlan, name string, fb *fileBuf) {
	if !g.claim(tu.name, f.Name+" (texture bytes form)") {
		return
	}
	id := jen.Id
	lit := jen.Lit(tu.name)
	single := len(tu.planes) == 1
	var params []jen.Code
	params = append(params, id(plans[0].name).Add(plans[0].pub), id(plans[1].name).Add(plans[1].pub))
	var slices []string
	for i := range tu.planes {
		sn := plans[2+2*i].name
		slices = append(slices, sn)
		params = append(params, id(sn).Index().Byte(), id(plans[3+2*i].name).Add(plans[3+2*i].pub))
	}
	tex, rect := id(plans[0].name), id(plans[1].name)

	var body []jen.Code
	// Empty buffers are a no-op, as for the other []byte forms.
	var allEmpty *jen.Statement
	for _, s := range slices {
		c := jen.Len(id(s)).Op("==").Lit(0)
		if allEmpty == nil {
			allEmpty = c
		} else {
			allEmpty = allEmpty.Op("&&").Add(c)
		}
	}
	body = append(body, jen.If(allEmpty).Block(jen.Return(jen.True())))
	if len(tu.formats) > 0 {
		var fm []jen.Code
		for _, c := range tu.formats {
			fm = append(fm, id(c))
		}
		body = append(body, jen.If(jen.Op("!").Id("textureHasFormat").Call(append([]jen.Code{lit, tex}, fm...)...)).Block(jen.Return(jen.False())))
	}
	body = append(body,
		jen.List(id("w"), id("h"), id("ok")).Op(":=").Id("textureUpdateRegion").Call(lit, tex, rect),
		jen.If(jen.Op("!").Id("ok")).Block(jen.Return(jen.False())),
	)
	for i, p := range tu.planes {
		w, h := id("w"), id("h")
		var wExpr, hExpr jen.Code = w, h
		if p.half {
			wExpr = jen.Parens(id("w").Op("+").Lit(1)).Op("/").Lit(2)
			hExpr = jen.Parens(id("h").Op("+").Lit(1)).Op("/").Lit(2)
		}
		if p.interleaved {
			wExpr = jen.Lit(2).Op("*").Parens(wExpr)
		}
		var bpp jen.Code = jen.Lit(1)
		if p.scaled {
			bpp = jen.Int64().Call(id("BYTESPERPIXEL").Call(tex.Clone().Dot("Format")))
		}
		var tail jen.Code = jen.Lit(0)
		if p.formatTail {
			tail = id("texturePlanarTail").Call(tex.Clone().Dot("Format"), hExpr, jen.Int64().Call(id(plans[3+2*i].name)))
		}
		body = append(body, jen.If(jen.Op("!").Id("textureCheckPlane").Call(
			lit, jen.Lit(slices[i]), jen.Len(id(slices[i])), id(plans[3+2*i].name), wExpr, hExpr, bpp, tail,
		)).Block(jen.Return(jen.False())))
	}
	var args []jen.Code
	args = append(args, tex, rect)
	for i := range tu.planes {
		pitch := id(plans[3+2*i].name)
		if single {
			args = append(args, jen.Qual("unsafe", "Pointer").Call(jen.Op("&").Id(slices[i]).Index(jen.Lit(0))), pitch)
		} else {
			args = append(args, id("bytesPtr").Call(id(slices[i])), pitch)
		}
	}
	body = append(body, id("ok").Op("=").Id(name).Call(args...))
	for _, s := range slices {
		body = append(body, jen.Qual("runtime", "KeepAlive").Call(id(s)))
	}
	body = append(body, jen.Return(id("ok")))

	doc := []string{tu.name + " is the []byte form of " + name + ": each pixel plane is a byte slice that holds the whole rectangle at the given pitch."}
	doc = append(doc, "")
	need := "pitch*(h-1)+w*bytesPerPixel bytes, where w and h are the size of rect (the whole texture when rect is nil)"
	if !single {
		need = "pitch*(rows-1)+rowBytes bytes for its rows, where the luma plane covers the w by h size of rect (the whole texture when rect is nil) and the chroma planes cover half of it, rounded up"
	}
	if tu.planes[0].formatTail {
		doc = append(doc, "For a planar texture format (YV12, IYUV, NV12, NV21, P010) SDL also reads the chroma planes from the same buffer, after the h rows of pitch bytes of the luma plane, so the slice must hold them as well: pitch*h plus 2*((h+1)/2)*((pitch+1)/2) bytes for YV12 and IYUV, or ((h+1)/2)*pitch bytes for the NV formats.", "")
	}
	doc = append(doc,
		"It sets the SDL error and returns false, without calling SDL, when the texture is nil, a pitch is not positive, rect has a negative size, or a slice is shorter than "+need+".")
	if len(tu.formats) > 0 {
		doc = append(doc, "It also fails when the texture format is not one that "+name+" accepts.")
	}
	if single {
		doc = append(doc, "An empty or nil slice is a no-op that returns true without calling SDL.")
	} else {
		doc = append(doc, "If every slice is empty it is a no-op that returns true without calling SDL; a single empty slice is only accepted for an empty rectangle.")
	}
	doc = append(doc, "", "See also: "+name)
	fb.funcs = append(fb.funcs, start(doc).Func().Id(tu.name).Params(params...).Bool().Block(body...))
}

// textureHelpers emits the pure-Go helpers the texture []byte forms call.
func textureHelpers(f *jen.File) {
	id := jen.Id
	f.Comment("textureHasFormat reports whether the texture has one of formats. Otherwise it")
	f.Comment("sets the SDL error to a message naming fn and returns false.")
	f.Func().Id("textureHasFormat").Params(id("fn").String(), id("texture").Op("*").Id("Texture"), id("formats").Op("...").Id("PixelFormat")).Bool().Block(
		jen.If(id("texture").Op("==").Nil()).Block(jen.Return(id("SetError").Call(jen.Lit("%s: texture is nil"), id("fn")))),
		jen.For(jen.List(jen.Id("_"), id("f")).Op(":=").Range().Id("formats")).Block(
			jen.If(id("texture").Dot("Format").Op("==").Id("f")).Block(jen.Return(jen.True()))),
		jen.Return(id("SetError").Call(jen.Lit("%s: unsupported texture format %#x"), id("fn"), id("texture").Dot("Format"))),
	)
	f.Comment("textureUpdateRegion returns the size of the rectangle an update covers: rect,")
	f.Comment("or the whole texture when rect is nil. It sets the SDL error to a message")
	f.Comment("naming fn and returns false when the texture is nil or the size is negative.")
	f.Func().Id("textureUpdateRegion").Params(id("fn").String(), id("texture").Op("*").Id("Texture"), id("rect").Op("*").Id("Rect")).Params(id("w").Int64(), id("h").Int64(), id("ok").Bool()).Block(
		jen.If(id("texture").Op("==").Nil()).Block(
			id("SetError").Call(jen.Lit("%s: texture is nil"), id("fn")), jen.Return(jen.Lit(0), jen.Lit(0), jen.False())),
		id("w").Op(",").Id("h").Op("=").Int64().Call(id("texture").Dot("W")).Op(",").Int64().Call(id("texture").Dot("H")),
		jen.If(id("rect").Op("!=").Nil()).Block(
			id("w").Op(",").Id("h").Op("=").Int64().Call(id("rect").Dot("W")).Op(",").Int64().Call(id("rect").Dot("H"))),
		jen.If(id("w").Op("<").Lit(0).Op("||").Id("h").Op("<").Lit(0)).Block(
			id("SetError").Call(jen.Lit("%s: negative rect size %dx%d"), id("fn"), id("w"), id("h")), jen.Return(jen.Lit(0), jen.Lit(0), jen.False())),
		jen.Return(id("w"), id("h"), jen.True()),
	)
	f.Comment("texturePlanarTail returns the number of bytes SDL_UpdateTexture reads after the")
	f.Comment("h rows of a single-plane buffer when format is planar: the chroma planes of YV12 and")
	f.Comment("IYUV, or the interleaved UV plane of NV12, NV21 and P010. It is zero for every other")
	f.Comment("format.")
	f.Func().Id("texturePlanarTail").Params(id("format").Id("PixelFormat"), id("h").Op(",").Id("pitch").Int64()).Int64().Block(
		jen.Switch(id("format")).Block(
			jen.Case(id("PIXELFORMAT_YV12"), id("PIXELFORMAT_IYUV")).Block(
				jen.Return(jen.Lit(2).Op("*").Parens(jen.Parens(id("h").Op("+").Lit(1)).Op("/").Lit(2)).Op("*").Parens(jen.Parens(id("pitch").Op("+").Lit(1)).Op("/").Lit(2)))),
			jen.Case(id("PIXELFORMAT_NV12"), id("PIXELFORMAT_NV21"), id("PIXELFORMAT_P010")).Block(
				jen.Return(jen.Parens(jen.Parens(id("h").Op("+").Lit(1)).Op("/").Lit(2)).Op("*").Id("pitch"))),
		),
		jen.Return(jen.Lit(0)),
	)
	f.Comment("textureCheckPlane checks one pixel plane of a texture update: the pitch must be")
	f.Comment("positive and the n bytes must hold h rows of w samples of bpp bytes at that pitch")
	f.Comment("(the last row needs only w*bpp bytes), plus tail bytes of further planes that follow")
	f.Comment("the h full rows. An empty rectangle needs no bytes.")
	f.Comment("Otherwise it sets the SDL error to a message naming fn and plane and returns false.")
	f.Func().Id("textureCheckPlane").Params(id("fn").Op(",").Id("plane").String(), id("n").Int(), id("pitch").Int32(), id("w").Op(",").Id("h").Op(",").Id("bpp").Op(",").Id("tail").Int64()).Bool().Block(
		jen.If(id("pitch").Op("<=").Lit(0)).Block(
			jen.Return(id("SetError").Call(jen.Lit("%s: %s pitch %d must be positive"), id("fn"), id("plane"), id("pitch")))),
		jen.If(id("w").Op("==").Lit(0).Op("||").Id("h").Op("==").Lit(0)).Block(jen.Return(jen.True())),
		id("need").Op(":=").Int64().Call(id("pitch")).Op("*").Parens(id("h").Op("-").Lit(1)).Op("+").Id("w").Op("*").Id("bpp"),
		jen.If(id("tail").Op(">").Lit(0)).Block(id("need").Op("=").Int64().Call(id("pitch")).Op("*").Id("h").Op("+").Id("tail")),
		jen.If(jen.Int64().Call(id("n")).Op("<").Id("need")).Block(
			jen.Return(id("SetError").Call(jen.Lit("%s: %s has %d bytes, need %d"), id("fn"), id("plane"), id("n"), id("need")))),
		jen.Return(jen.True()),
	)
	f.Comment("bytesPtr returns the address of the first byte of b, or nil when b is empty.")
	f.Func().Id("bytesPtr").Params(id("b").Index().Byte()).Op("*").Id("uint8").Block(
		jen.If(jen.Len(id("b")).Op("==").Lit(0)).Block(jen.Return(jen.Nil())),
		jen.Return(jen.Op("&").Id("b").Index(jen.Lit(0))),
	)
}
