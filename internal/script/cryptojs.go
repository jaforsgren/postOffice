package script

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"github.com/dop251/goja"
)

// SetupCryptoJS registers CryptoJS, atob, and btoa on the given VM.
// It is exported so the workflow runtime can equip its own VM with the same globals.
func SetupCryptoJS(vm *goja.Runtime) error { return setupCryptoJS(vm) }

func setupCryptoJS(vm *goja.Runtime) error {
	newWordArray := func(data []byte) goja.Value {
		obj := vm.NewObject()
		if err := obj.Set("_data", vm.NewArrayBuffer(data)); err != nil {
			return goja.Undefined()
		}
		return obj
	}

	extractBytes := func(val goja.Value) ([]byte, bool) {
		if val == nil || goja.IsUndefined(val) || goja.IsNull(val) {
			return nil, false
		}
		if exported := val.Export(); exported != nil {
			if str, ok := exported.(string); ok {
				return []byte(str), true
			}
		}
		obj := val.ToObject(vm)
		dataVal := obj.Get("_data")
		if dataVal == nil || goja.IsUndefined(dataVal) || goja.IsNull(dataVal) {
			return nil, false
		}
		ab, ok := dataVal.Export().(goja.ArrayBuffer)
		if !ok {
			return nil, false
		}
		return ab.Bytes(), true
	}

	utf8Obj := vm.NewObject()
	if err := utf8Obj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		return newWordArray([]byte(call.Arguments[0].String()))
	}); err != nil {
		return fmt.Errorf("CryptoJS.enc.Utf8.parse: %w", err)
	}

	base64Obj := vm.NewObject()
	if err := base64Obj.Set("stringify", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		data, ok := extractBytes(call.Arguments[0])
		if !ok {
			return goja.Undefined()
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString(data))
	}); err != nil {
		return fmt.Errorf("CryptoJS.enc.Base64.stringify: %w", err)
	}
	if err := base64Obj.Set("parse", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		decoded, err := base64.StdEncoding.DecodeString(call.Arguments[0].String())
		if err != nil {
			return goja.Undefined()
		}
		return newWordArray(decoded)
	}); err != nil {
		return fmt.Errorf("CryptoJS.enc.Base64.parse: %w", err)
	}

	encObj := vm.NewObject()
	if err := encObj.Set("Utf8", utf8Obj); err != nil {
		return fmt.Errorf("CryptoJS.enc.Utf8: %w", err)
	}
	if err := encObj.Set("Base64", base64Obj); err != nil {
		return fmt.Errorf("CryptoJS.enc.Base64: %w", err)
	}

	cryptoJSObj := vm.NewObject()
	if err := cryptoJSObj.Set("enc", encObj); err != nil {
		return fmt.Errorf("CryptoJS.enc: %w", err)
	}

	if err := cryptoJSObj.Set("HmacSHA256", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 2 {
			return goja.Undefined()
		}
		msg, okMsg := extractBytes(call.Arguments[0])
		key, okKey := extractBytes(call.Arguments[1])
		if !okMsg || !okKey {
			return goja.Undefined()
		}
		mac := hmac.New(sha256.New, key)
		mac.Write(msg)
		return newWordArray(mac.Sum(nil))
	}); err != nil {
		return fmt.Errorf("CryptoJS.HmacSHA256: %w", err)
	}

	if err := vm.Set("CryptoJS", cryptoJSObj); err != nil {
		return fmt.Errorf("failed to set CryptoJS global: %w", err)
	}

	// atob / btoa — standard browser globals for base64 encoding/decoding.
	if err := vm.Set("atob", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		decoded, err := base64.StdEncoding.DecodeString(call.Arguments[0].String())
		if err != nil {
			panic(vm.NewGoError(fmt.Errorf("atob: invalid base64: %w", err)))
		}
		return vm.ToValue(string(decoded))
	}); err != nil {
		return fmt.Errorf("failed to set atob: %w", err)
	}

	if err := vm.Set("btoa", func(call goja.FunctionCall) goja.Value {
		if len(call.Arguments) < 1 {
			return goja.Undefined()
		}
		return vm.ToValue(base64.StdEncoding.EncodeToString([]byte(call.Arguments[0].String())))
	}); err != nil {
		return fmt.Errorf("failed to set btoa: %w", err)
	}

	return nil
}
