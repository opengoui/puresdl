package bindgen

import (
	"math/big"
	"regexp"
	"sort"
	"strings"

	"github.com/opengoui/puresdl/internal/ir"
)

// keptStdinc are the SDL_stdinc.h functions that have no Go standard library
// equivalent and are therefore bound. Every other function declared there is
// a libc replacement (string, math, memory, wide-char helpers) that Go
// programs should not call through SDL.
var keptStdinc = regexp.MustCompile(`^SDL_(` +
	`malloc|calloc|realloc|free|aligned_alloc|aligned_free|GetOriginalMemoryFunctions|GetMemoryFunctions|SetMemoryFunctions|GetNumAllocations|` +
	`CreateEnvironment|GetEnvironment|GetEnvironmentVariable|GetEnvironmentVariables|SetEnvironmentVariable|UnsetEnvironmentVariable|DestroyEnvironment|` +
	`getenv|getenv_unsafe|setenv_unsafe|unsetenv_unsafe|` +
	`srand|rand|randf|rand_bits|rand_r|randf_r|rand_bits_r|murmur3_32|crc16|crc32|` +
	`iconv_open|iconv_close|iconv|iconv_string` +
	`)$`)

// skipTypes are helper declarations of the headers that are not API.
var skipTypes = map[string]bool{"SDL_alignment_test": true, "SDL_DUMMY_ENUM": true}

var nullRe = regexp.MustCompile(`\bNULL\b`)

// nullable reports whether a \param description allows NULL, ignoring
// "must not be NULL" and "NULL-terminated".
func nullable(doc string) bool {
	for _, m := range nullRe.FindAllStringIndex(doc, -1) {
		before := strings.ToLower(doc[max(0, m[0]-16):m[0]])
		after := doc[m[1]:min(len(doc), m[1]+12)]
		if strings.Contains(before, "not ") || strings.Contains(before, "non-") || strings.HasPrefix(after, "-terminated") {
			continue
		}
		return true
	}
	return false
}

// freedStringRe recognises documentation that tells the caller to release a
// returned allocation with SDL_free.
var freedStringRe = regexp.MustCompile(`SDL_free\b`)

// paramTypeOverrides replace the Go type of C parameters that are declared as
// plain integers although they carry an enum value.
var paramTypeOverrides = map[string]string{
	"EventEnabled.type": "EventType", "HasEvent.type": "EventType", "HasEvents.minType": "EventType", "HasEvents.maxType": "EventType", "SetEventEnabled.type": "EventType", "FlushEvent.type": "EventType",
	"FlushEvents.minType": "EventType", "FlushEvents.maxType": "EventType",
	"PeepEvents.minType": "EventType", "PeepEvents.maxType": "EventType",
}

// fieldTypeOverrides do the same for struct and union members.
var fieldTypeOverrides = map[string]string{"Event.type": "EventType", "CommonEvent.type": "EventType"}

// initialisms are spelled in capitals when they start a lower-case C name.
var initialisms = map[string]string{"hid": "HID", "id": "ID", "io": "IO", "utf": "UTF", "crc": "CRC"}

// handWritten functions get only their raw function variable (fn<Name>); the
// exported wrapper lives in a hand-written file because a generic rule would
// be wrong for them.
var handWritten = map[string]bool{
	"SDL_GetPreferredLocales": true,
	// SDL keeps the name pointer; the wrapper has to own a permanent copy.
	"SDL_SetScancodeName": true,
	// The output has no length and the input length must derive from the Go value.
	"SDL_iconv_string": true,
}

// constTypeOverrides pins the type of macro constants whose contiguous
// #define block does not name the right typedef. The value is the C typedef
// name; "" keeps the constant untyped. Entries were checked against the SDL
// 3.5 headers.
var constTypeOverrides = map[string]string{
	// Positions, not window flags: they are passed as int coordinates.
	"SDL_WINDOWPOS_UNDEFINED_MASK": "",
	"SDL_WINDOWPOS_UNDEFINED":      "",
	"SDL_WINDOWPOS_CENTERED_MASK":  "",
	"SDL_WINDOWPOS_CENTERED":       "",
	// Sentinel IDs follow the ID typedef they are cast to in the headers,
	// not the (SDL_PenID, SDL_Finger) declaration that precedes them.
	"SDL_PEN_MOUSEID":   "SDL_MouseID",
	"SDL_PEN_TOUCHID":   "SDL_TouchID",
	"SDL_TOUCH_MOUSEID": "SDL_MouseID",
	"SDL_MOUSE_TOUCHID": "SDL_TouchID",
	// SDL_HAPTIC_INFINITY is an effect length, not part of SDL_Haptic.
	"SDL_HAPTIC_INFINITY": "",
	// Button numbers are indices that are compared with the uint8 Button field
	// of the mouse events, so they stay untyped; only the masks are
	// SDL_MouseButtonFlags.
	"SDL_BUTTON_LEFT":   "",
	"SDL_BUTTON_MIDDLE": "",
	"SDL_BUTTON_RIGHT":  "",
	"SDL_BUTTON_X1":     "",
	"SDL_BUTTON_X2":     "",
	"SDL_BUTTON_LMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_MMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_RMASK":  "SDL_MouseButtonFlags",
	"SDL_BUTTON_X1MASK": "SDL_MouseButtonFlags",
	"SDL_BUTTON_X2MASK": "SDL_MouseButtonFlags",
}

// constType returns the Go type name for an integer macro, or "" when it
// stays untyped. A macro takes the integer typedef declared immediately
// before its #define block (ir.Const.After) unless overridden, and only
// when that typedef is emitted as a named Go type that can hold the value.
func (g *gen) constType(k *ir.Const) string {
	cName := k.After
	if o, ok := constTypeOverrides[k.Name]; ok {
		cName = o
	}
	if cName == "" {
		return ""
	}
	n, ok := g.tdGo[cName]
	if !ok {
		return ""
	}
	td := g.typedefs[cName]
	res := td.Type.Under.Resolve()
	if res == nil || res.Kind != ir.Int || res.Char {
		return ""
	}
	if !fitsInt(k.Int, res.Size, res.Signed) {
		return ""
	}
	return n
}

// fitsInt reports whether v is representable in an integer of size bytes.
func fitsInt(v *big.Int, size int64, signed bool) bool {
	bitsN := uint(size * 8)
	if signed {
		lim := new(big.Int).Lsh(big.NewInt(1), bitsN-1)
		return v.Cmp(new(big.Int).Neg(lim)) >= 0 && v.Cmp(lim) < 0
	}
	return v.Sign() >= 0 && v.BitLen() <= int(bitsN)
}

// nullParam classifies a const char * parameter whose \param documentation
// allows NULL.
type nullParam struct {
	// distinct is true when NULL means something different from an empty
	// string; the public parameter is then a *string (nil is NULL). Otherwise
	// the parameter stays a string and "" is passed as NULL.
	distinct bool
	// doc quotes the sentence of the SDL documentation that allows NULL.
	doc string
	// effect completes "A nil <param> ..." in the generated doc comment of a
	// distinct parameter.
	effect string
}

// nullParams classifies every nullable const char * parameter, keyed by
// "C function.parameter". bindFunc and CheckNullPolicy fail on a nullable
// parameter missing here, so an SDL upgrade that adds one is reviewed.
var nullParams = map[string]nullParam{
	// NULL deletes the property; "" stores an empty string.
	"SDL_SetStringProperty.value":      {true, "the new value of the property, or NULL to delete the property.", "deletes the property"},
	"SDL_SetAppMetadataProperty.value": {true, "the value of the property, or NULL to remove that property.", "removes the property"},
	"SDL_SetGamepadMapping.mapping":    {true, "the mapping to use for this device, or NULL to clear the mapping.", "clears the mapping"},
	// NULL makes a separator; "" is an entry with an empty label.
	// NULL selects the default title root (SDL_OpenTitleStorage(NULL, 0)); ""
	// is an empty override. The header documents neither, see undocumentedNull.
	"SDL_OpenTitleStorage.override": {true, "a path to override the backend's default title root.", "selects the default title root"},
	"SDL_InsertTrayEntryAt.label":   {true, "the text to be displayed on the entry, in UTF-8 encoding, or NULL for a separator.", "inserts a separator"},

	// The remaining parameters select a default; "" and NULL are interchangeable.
	"SDL_GL_LoadLibrary.path":                   {doc: "the platform dependent OpenGL library name, or NULL to open the default OpenGL library."},
	"SDL_Vulkan_LoadLibrary.path":               {doc: "the platform dependent Vulkan loader library name or NULL."},
	"SDL_ShowOpenFileDialog.default_location":   {doc: "the default folder or file to start the dialog at, may be NULL."},
	"SDL_ShowSaveFileDialog.default_location":   {doc: "the default folder or file to start the dialog at, may be NULL."},
	"SDL_ShowOpenFolderDialog.default_location": {doc: "the default folder or file to start the dialog at, may be NULL."},
	"SDL_ShowNotification.message":              {doc: "UTF-8 message text, may be NULL."},
	"SDL_GlobDirectory.pattern":                 {doc: "the pattern that files in the directory must match. Can be NULL."},
	"SDL_GlobStorageDirectory.pattern":          {doc: "the pattern that files in the directory must match. Can be NULL."},
	"SDL_GPUSupportsShaderFormats.name":         {doc: "the preferred GPU driver, or NULL to let SDL pick the optimal driver."},
	"SDL_CreateGPUDevice.name":                  {doc: "the preferred GPU driver, or NULL to let SDL pick the optimal driver."},
	"SDL_CreateRenderer.name":                   {doc: "the name of the rendering driver to initialize, or NULL to let SDL choose one."},
	// An empty prefix prints the same as no prefix.
	"SDL_SetLogPriorityPrefix.prefix": {doc: "the prefix to use for that log priority, or NULL to use no prefix."},
	// An empty base path is the same as no base path; an empty directory is the root.
	"SDL_OpenFileStorage.path":           {doc: "the base path prepended to all storage paths, or NULL for no base path."},
	"SDL_EnumerateStorageDirectory.path": {doc: "the path of the directory to enumerate, or NULL for the root."},
	"SDL_GlobStorageDirectory.path":      {doc: "the path of the directory to enumerate, or NULL for the root."},
	"SDL_CreateTray.tooltip":             {doc: "a tooltip to be displayed when the mouse hovers the icon in UTF-8 encoding. Not supported on all platforms. May be NULL."},
	"SDL_SetTrayTooltip.tooltip":         {doc: "the new tooltip in UTF-8 encoding. May be NULL."},
	"SDL_RegisterApp.name":               {doc: "the window class name, in UTF-8 encoding. If NULL, SDL currently uses \"SDL_app\" but this isn't guaranteed."},
}

// undocumentedNull lists the nullParams whose \param text does not mention
// NULL although the header's examples and the implementation accept it.
var undocumentedNull = map[string]bool{"SDL_OpenTitleStorage.override": true}

// nullDistinctResults are the string results where NULL means "not set" and
// is different from an empty string. Each gets an extra exported variant
// <Name>OK returning (string, bool); the plain function keeps returning "" for
// NULL so existing callers are unaffected. Keys are C function names, values
// the documentation sentence.
var nullDistinctResults = map[string]string{
	"SDL_GetHint":                "the string value of a hint or NULL if the hint isn't set.",
	"SDL_GetEnvironmentVariable": "a pointer to the value of the variable or NULL if it can't be found.",
	"SDL_getenv":                 "a pointer to the value of the variable or NULL if it can't be found.",
	"SDL_getenv_unsafe":          "a pointer to the value of the variable or NULL if it can't be found.",
	"SDL_GetAppMetadataProperty": "the current value of the metadata property, or the default if it is not set, NULL for properties with no default.",
}

// nullFailureResults are the string results whose documentation allows NULL
// only for failure, a missing optional item or an invalid index. "" carries
// the same information (and SDL_GetError explains failures), so the plain
// string result stays.
var nullFailureResults = map[string]string{
	"SDL_iconv_string":                          "a new string, converted to the new encoding, or NULL on error.",
	"SDL_GetThreadName":                         "a pointer to a UTF-8 string that names the specified thread, or NULL if it doesn't have a name.",
	"SDL_GetAudioDriver":                        "the name of the audio driver at the requested index, or NULL if an invalid index was specified.",
	"SDL_GetCurrentAudioDriver":                 "the name of the current audio driver or NULL if no driver has been initialized.",
	"SDL_GetAudioDeviceName":                    "the name of the audio device, or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetCameraDriver":                       "the name of the camera driver at the requested index, or NULL if an invalid index was specified.",
	"SDL_GetCurrentCameraDriver":                "the name of the current camera driver or NULL if no driver has been initialized.",
	"SDL_GetCameraName":                         "a human-readable device name or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetVideoDriver":                        "the name of the video driver with the given **index**, or NULL if index is out of bounds.",
	"SDL_GetCurrentVideoDriver":                 "the name of the current video driver or NULL if no driver has been initialized.",
	"SDL_GetDisplayName":                        "the name of a display or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetSensorNameForID":                    "the sensor name, or NULL if `instance_id` is not valid.",
	"SDL_GetSensorName":                         "the sensor name or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetJoystickNameForID":                  "the name of the selected joystick. If no name can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetJoystickPathForID":                  "the path of the selected joystick. If no path can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetJoystickName":                       "the name of the selected joystick. If no name can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetJoystickPath":                       "the path of the selected joystick. If no path can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetJoystickSerial":                     "the serial number of the selected joystick, or NULL if unavailable.",
	"SDL_GetGamepadMappingForGUID":              "a mapping string or NULL on failure; call SDL_GetError() for more information. This should be freed with SDL_free() when it is no longer needed.",
	"SDL_GetGamepadMapping":                     "a string that has the gamepad's mapping or NULL if no mapping is available; call SDL_GetError() for more information. This should be freed with SDL_free() when it is no longer needed.",
	"SDL_GetGamepadNameForID":                   "the name of the selected gamepad. If no name can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetGamepadPathForID":                   "the path of the selected gamepad. If no path can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetGamepadMappingForID":                "the mapping string. Returns NULL if no mapping is available. This should be freed with SDL_free() when it is no longer needed.",
	"SDL_GetGamepadName":                        "the implementation dependent name for the gamepad, or NULL if there is no name or the identifier passed is invalid.",
	"SDL_GetGamepadPath":                        "the implementation dependent path for the gamepad, or NULL if there is no path or the identifier passed is invalid.",
	"SDL_GetGamepadSerial":                      "the serial number, or NULL if unavailable.",
	"SDL_GetGamepadStringForType":               "a string for the given type, or NULL if an invalid type is specified. The string returned is of the format used by SDL_Gamepad mapping strings.",
	"SDL_GetGamepadStringForAxis":               "a string for the given axis, or NULL if an invalid axis is specified. The string returned is of the format used by SDL_Gamepad mapping strings.",
	"SDL_GetGamepadStringForButton":             "a string for the given button, or NULL if an invalid button is specified. The string returned is of the format used by SDL_Gamepad mapping strings.",
	"SDL_GetGamepadAppleSFSymbolsNameForButton": "the sfSymbolsName or NULL if the name can't be found.",
	"SDL_GetGamepadAppleSFSymbolsNameForAxis":   "the sfSymbolsName or NULL if the name can't be found.",
	"SDL_GetKeyboardNameForID":                  "the name of the selected keyboard or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetMouseNameForID":                     "the name of the selected mouse, or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetTouchDeviceName":                    "touch device name, or NULL on failure; call SDL_GetError() for more information.",
	"SDL_GetBasePath":                           "an absolute path in UTF-8 encoding to the application data directory. NULL will be returned on error or when the platform doesn't implement this functionality, call SDL_GetError() for more information.",
	"SDL_GetPrefPath":                           "a UTF-8 string of the user directory in platform-dependent notation. NULL if there's a problem (creating directory failed, etc.). This should be freed with SDL_free() when it is no longer needed.",
	"SDL_GetUserFolder":                         "either a null-terminated C string containing the full path to the folder, or NULL if an error happened.",
	"SDL_GetCurrentDirectory":                   "a UTF-8 string of the current working directory in platform-dependent notation. NULL if there's a problem. This should be freed with SDL_free() when it is no longer needed.",
	"SDL_GetGPUDriver":                          "the name of the GPU driver with the given **index** or NULL when the index is out of bounds; call SDL_GetError() for more information.",
	"SDL_GetGPUDeviceDriver":                    "the name of the device's driver, or NULL on error.",
	"SDL_GetHapticNameForID":                    "the name of the selected haptic device. If no name can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetHapticName":                         "the name of the selected haptic device. If no name can be found, this function returns NULL; call SDL_GetError() for more information.",
	"SDL_GetRenderDriver":                       "the name of the rendering driver at the requested index, or NULL if an invalid index was specified.",
	"SDL_GetRendererName":                       "the name of the selected renderer, or NULL on failure; call SDL_GetError() for more information.",
}

// returnsDoc extracts the \returns paragraph of a function documentation.
func returnsDoc(doc string) string {
	_, rest, ok := strings.Cut(doc, "\\returns")
	if !ok {
		return ""
	}
	if i := strings.Index(rest, "\\"); i >= 0 {
		rest = rest[:i]
	}
	return rest
}

// returnsNull reports whether a \returns paragraph mentions NULL as a possible
// result. Unlike nullable it does not look for "not" before the word, since
// "if it is not set, NULL" is a NULL result.
func returnsNull(doc string) bool {
	for _, m := range nullRe.FindAllStringIndex(doc, -1) {
		if !strings.HasPrefix(doc[m[1]:], "-terminated") {
			return true
		}
	}
	return false
}

// CheckNullPolicy returns one message per nullable const char * parameter or
// NULL-able string result of api that is missing from the classification
// tables, and per table entry that no longer matches a declaration.
func CheckNullPolicy(api *ir.API) []string {
	var out []string
	seenP, seenR := map[string]bool{}, map[string]bool{}
	for _, f := range api.Funcs {
		if f.File == "SDL_stdinc.h" && !keptStdinc.MatchString(f.Name) {
			continue
		}
		for _, p := range f.Sig.Params {
			if k := f.Name + "." + p.Name; isCString(p.Type) && p.Type.Resolve().Const && (nullable(p.Doc) || undocumentedNull[k]) {
				seenP[k] = true
				if _, ok := nullParams[k]; !ok {
					out = append(out, "unclassified nullable parameter "+k+" (add it to nullParams): "+strings.Join(strings.Fields(p.Doc), " "))
				}
			}
		}
		if isCString(f.Sig.Ret) && returnsNull(returnsDoc(f.Doc)) {
			seenR[f.Name] = true
			_, a := nullDistinctResults[f.Name]
			_, b := nullFailureResults[f.Name]
			if !a && !b {
				out = append(out, "unclassified NULL-able string result "+f.Name+" (add it to nullDistinctResults or nullFailureResults): "+strings.Join(strings.Fields(returnsDoc(f.Doc)), " "))
			}
		}
	}
	for k := range nullParams {
		if !seenP[k] {
			out = append(out, "stale nullParams entry "+k)
		}
	}
	for _, m := range []map[string]string{nullDistinctResults, nullFailureResults} {
		for k := range m {
			if !seenR[k] {
				out = append(out, "stale result entry "+k)
			}
		}
	}
	sort.Strings(out)
	return out
}
