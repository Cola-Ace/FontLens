package fonts

import (
	"context"
	"fmt"
	"runtime"
	"unicode/utf16"
	"unsafe"

	"github.com/deploymenttheory/go-bindings-win32/bindings/runtime/win32"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/foundation"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/graphics/directwrite"
	"github.com/deploymenttheory/go-bindings-win32/bindings/win32/system/com"
)

func SystemFaces(ctx context.Context, visit func(string, int), report func(error)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if _, err := com.CoInitializeEx(uint32(com.COINIT_MULTITHREADED)); err != nil {
		return err
	}
	defer com.CoUninitialize()
	var unknown *win32.IUnknown
	if err := directwrite.DWriteCreateFactory(directwrite.DWRITE_FACTORY_TYPE_ISOLATED, &directwrite.IID_IDWriteFactory, &unknown); err != nil {
		return err
	}
	factory := (*directwrite.IDWriteFactory)(unsafe.Pointer(unknown))
	defer factory.Release()
	var collection *directwrite.IDWriteFontCollection
	if err := factory.GetSystemFontCollection(&collection, false); err != nil {
		return err
	}
	defer collection.Release()
	for familyIndex := uint32(0); familyIndex < collection.GetFontFamilyCount(); familyIndex++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		var family *directwrite.IDWriteFontFamily
		if err := collection.GetFontFamily(familyIndex, &family); err != nil {
			report(fmt.Errorf("DirectWrite family %d: %w", familyIndex, err))
			continue
		}
		func() {
			defer family.Release()
			list, err := win32.QueryInterface[directwrite.IDWriteFontList1](family, &directwrite.IID_IDWriteFontList1)
			if err != nil {
				report(fmt.Errorf("DirectWrite family %d face list: %w", familyIndex, err))
				return
			}
			defer list.Release()
			for faceIndex := uint32(0); faceIndex < family.GetFontCount(); faceIndex++ {
				if ctx.Err() != nil {
					return
				}
				path, collectionIndex, err := systemFacePath(list, faceIndex)
				if err != nil {
					report(fmt.Errorf("DirectWrite family %d face %d: %w", familyIndex, faceIndex, err))
					continue
				}
				visit(path, collectionIndex)
			}
		}()
	}
	return ctx.Err()
}

func systemFacePath(list *directwrite.IDWriteFontList1, index uint32) (string, int, error) {
	var reference *directwrite.IDWriteFontFaceReference
	if err := list.GetFontFaceReference(index, &reference); err != nil {
		return "", 0, err
	}
	defer reference.Release()
	var file *directwrite.IDWriteFontFile
	if err := reference.GetFontFile(&file); err != nil {
		return "", 0, err
	}
	defer file.Release()
	var loader *directwrite.IDWriteFontFileLoader
	if err := file.GetLoader(&loader); err != nil {
		return "", 0, err
	}
	defer loader.Release()
	local, err := win32.QueryInterface[directwrite.IDWriteLocalFontFileLoader](loader, &directwrite.IID_IDWriteLocalFontFileLoader)
	if err != nil {
		return "", 0, fmt.Errorf("font file is not local: %w", err)
	}
	defer local.Release()
	var keyPointer unsafe.Pointer
	var keySize uint32
	if err := file.GetReferenceKey(&keyPointer, &keySize); err != nil {
		return "", 0, err
	}
	if keyPointer == nil || keySize == 0 || keySize > 1<<20 {
		return "", 0, fmt.Errorf("invalid font file reference key")
	}
	key := unsafe.Slice((*byte)(keyPointer), keySize)
	var length uint32
	if err := local.GetFilePathLengthFromKey(key, &length); err != nil {
		return "", 0, err
	}
	if length == 0 || length > 32767 {
		return "", 0, fmt.Errorf("invalid font file path length")
	}
	buffer := make([]uint16, length+1)
	if err := local.GetFilePathFromKey(key, foundation.PWSTR(&buffer[0]), uint32(len(buffer))); err != nil {
		return "", 0, err
	}
	return string(utf16.Decode(buffer[:length])), int(reference.GetFontFaceIndex()), nil
}
