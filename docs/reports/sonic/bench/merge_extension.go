package bench

import (
	"encoding/json"
	"reflect"
	"unsafe"

	jsoniter "github.com/json-iterator/go"
	"github.com/modern-go/reflect2"
	jsonpatch "gopkg.in/evanphx/json-patch.v5"
)

// mergeCloneExtensionStub is a faithful copy of util/jsonutil's
// mergeCloneExtension, kept here to model the cost of the merge-clone
// path independently of the main module. It is NOT used by the core
// codecs unless their config registers it (`jsoniter/merge-clone`).
//
// We keep this in the bench module — not as a dependency on the parent
// module — so the bench can stand on its own (separate go.mod).
type mergeCloneExtensionStub struct {
	jsoniter.DummyExtension
}

var jsonRawMessageType = reflect2.TypeOfPtr((*json.RawMessage)(nil)).Elem()

func (e *mergeCloneExtensionStub) CreateDecoder(typ reflect2.Type) jsoniter.ValDecoder {
	if typ == jsonRawMessageType {
		return &extMergeDecoder{sliceType: typ.(*reflect2.UnsafeSliceType)}
	}
	return nil
}

func (e *mergeCloneExtensionStub) DecorateDecoder(typ reflect2.Type, decoder jsoniter.ValDecoder) jsoniter.ValDecoder {
	if typ.Kind() == reflect.Ptr {
		ptrType := typ.(*reflect2.UnsafePtrType)
		return &ptrCloneDecoder{valueDecoder: decoder, elemType: ptrType.Elem()}
	}
	if typ.Kind() == reflect.Slice && typ != jsonRawMessageType {
		return &sliceCloneDecoder{valueDecoder: decoder, sliceType: typ.(*reflect2.UnsafeSliceType)}
	}
	if typ.Kind() == reflect.Map {
		return &mapCloneDecoder{valueDecoder: decoder, mapType: typ.(*reflect2.UnsafeMapType)}
	}
	return decoder
}

type extMergeDecoder struct{ sliceType *reflect2.UnsafeSliceType }

func (d *extMergeDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	if iter.ReadNil() {
		return
	}
	existing := *((*json.RawMessage)(ptr))
	incoming := iter.SkipAndReturnBytes()
	if iter.Error != nil {
		return
	}
	if len(existing) == 0 {
		*((*json.RawMessage)(ptr)) = incoming
		return
	}
	merged, err := jsonpatch.MergePatch(existing, incoming)
	if err != nil {
		iter.ReportError("merge", err.Error())
		return
	}
	*((*json.RawMessage)(ptr)) = merged
}

type ptrCloneDecoder struct {
	elemType     reflect2.Type
	valueDecoder jsoniter.ValDecoder
}

func (d *ptrCloneDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	if iter.ReadNil() {
		*((*unsafe.Pointer)(ptr)) = nil
		return
	}
	if *((*unsafe.Pointer)(ptr)) != nil {
		obj := d.elemType.UnsafeNew()
		d.elemType.UnsafeSet(obj, *((*unsafe.Pointer)(ptr)))
		*((*unsafe.Pointer)(ptr)) = obj
	}
	d.valueDecoder.Decode(ptr, iter)
}

type sliceCloneDecoder struct {
	sliceType    *reflect2.UnsafeSliceType
	valueDecoder jsoniter.ValDecoder
}

func (d *sliceCloneDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	d.sliceType.UnsafeSetNil(ptr)
	if iter.ReadNil() {
		return
	}
	d.valueDecoder.Decode(ptr, iter)
}

type mapCloneDecoder struct {
	mapType      *reflect2.UnsafeMapType
	valueDecoder jsoniter.ValDecoder
}

func (d *mapCloneDecoder) Decode(ptr unsafe.Pointer, iter *jsoniter.Iterator) {
	if iter.ReadNil() {
		*(*unsafe.Pointer)(ptr) = nil
		d.mapType.UnsafeSet(ptr, d.mapType.UnsafeNew())
		return
	}
	if !d.mapType.UnsafeIsNil(ptr) {
		clone := d.mapType.UnsafeMakeMap(0)
		mapIter := d.mapType.UnsafeIterate(ptr)
		for mapIter.HasNext() {
			key, elem := mapIter.UnsafeNext()
			d.mapType.UnsafeSetIndex(clone, key, elem)
		}
		d.mapType.UnsafeSet(ptr, clone)
	}
	d.valueDecoder.Decode(ptr, iter)
}
