package bindgen

import (
	"github.com/dave/jennifer/jen"

	"github.com/opengoui/puresdl/internal/ir"
)

// planarForm describes an audio function that takes an array of per-channel
// plane pointers (void **) plus a channel count and a per-plane sample count.
// The headers cannot say that the sample count is per plane or that the byte
// size of a sample derives from the stream's source format, so the functions
// are listed explicitly.
type planarForm struct {
	planesParam  string // the void ** parameter
	countParam   string // the int32 plane count
	samplesParam string // the int32 per-plane sample count
	streamParam  string // the *AudioStream whose source format sizes a sample
	name         string // the exported Go name of the slice form
}

// planarForms maps C function names to their planar slice form.
var planarForms = map[string]planarForm{
	"SDL_PutAudioStreamPlanarData": {
		planesParam: "channel_buffers", countParam: "num_channels",
		samplesParam: "num_samples", streamParam: "stream",
		name: "PutAudioStreamPlanarDataSlices",
	},
}

// planarFormOf returns the planar slice form of f, or nil when f is not
// listed or its parameters no longer match the table.
func planarFormOf(f *ir.Function, plans []argPlan) *planarForm {
	pf, ok := planarForms[f.Name]
	if !ok || len(plans) != 4 || f.Sig.Ret.Resolve().Kind != ir.Bool {
		return nil
	}
	for i, want := range []string{pf.streamParam, pf.planesParam, pf.countParam, pf.samplesParam} {
		if f.Sig.Params[i].Name != want || plans[i].hidden || plans[i].pub == nil {
			return nil
		}
	}
	return &pf
}

// planarNote is the doc note of the raw wrapper that points at its planar
// slice form.
func planarNote(pf *planarForm) string {
	return "The plane pointers and the count must agree and every non-NULL plane must hold the samples SDL reads; " + pf.name + " takes byte slices and checks them."
}

// emitPlanar emits the planar slice form of f. The form validates the plane
// lengths against the stream's source format before SDL is called.
func (g *gen) emitPlanar(f *ir.Function, pf *planarForm, plans []argPlan, name string, fb *fileBuf) {
	if !g.claim(pf.name, f.Name+" (planar slice form)") {
		return
	}
	id := jen.Id
	lit := jen.Lit(pf.name)
	body := []jen.Code{
		jen.If(jen.Len(id("planes")).Op("==").Lit(0)).Block(jen.Return(jen.True())),
		jen.If(id("numSamples").Op("<").Lit(0)).Block(
			jen.Return(id("SetError").Call(jen.Lit(pf.name+": negative sample count %d"), id("numSamples")))),
		jen.If(jen.Op("!").Id("fitsInt32").Call(jen.Len(id("planes")), lit)).Block(jen.Return(jen.False())),
		jen.Var().Id("src").Op(",").Id("dst").Id("AudioSpec"),
		jen.If(jen.Op("!").Id("GetAudioStreamFormat").Call(id("stream"), jen.Op("&").Id("src"), jen.Op("&").Id("dst"))).Block(jen.Return(jen.False())),
		id("need").Op(":=").Int64().Call(id("numSamples")).Op("*").Int64().Call(id("AUDIO_BYTESIZE").Call(id("src").Dot("Format"))),
		jen.For(id("i").Op(",").Id("p").Op(":=").Range().Id("planes")).Block(
			jen.If(jen.Len(id("p")).Op("==").Lit(0)).Block(jen.Continue()),
			jen.If(jen.Int64().Call(jen.Len(id("p"))).Op("<").Id("need")).Block(
				jen.Return(id("SetError").Call(jen.Lit(pf.name+": plane %d has %d bytes, need %d"), id("i"), jen.Len(id("p")), id("need"))))),
		id("ptrs").Op(":=").Id("planarPtrs").Call(id("planes")),
		id("ok").Op(":=").Id(name).Call(id("stream"), jen.Op("&").Id("ptrs").Index(jen.Lit(0)), jen.Int32().Call(jen.Len(id("planes"))), id("numSamples")),
		jen.Qual("runtime", "KeepAlive").Call(id("planes")),
		jen.Qual("runtime", "KeepAlive").Call(id("ptrs")),
		jen.Return(id("ok")),
	}
	doc := []string{
		pf.name + " queues planar audio like " + name + ", with one byte slice per channel, each holding numSamples samples of the stream's source format (AUDIO_BYTESIZE of the format from GetAudioStreamFormat bytes each).",
		"",
		"It returns false with an SDL error, without calling SDL's put function, when numSamples is negative, the stream format cannot be read, or any non-empty plane is shorter than numSamples samples, so SDL never reads past a plane. A nil or empty plane is passed as NULL and SDL writes silence for that channel. With no planes it is a no-op that returns true.",
		"",
		"See also: " + name,
	}
	fb.funcs = append(fb.funcs, start(doc).Func().Id(pf.name).Params(
		id("stream").Add(plans[0].pub), id("planes").Index().Index().Byte(), id("numSamples").Int32(),
	).Bool().Block(body...))
}
