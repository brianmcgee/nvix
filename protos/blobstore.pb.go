// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.31.0
// 	protoc        v4.24.2
// source: protos/blobstore.proto

package protos

import (
	reflect "reflect"
	sync "sync"

	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type BlobMeta struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Chunks []*BlobMeta_ChunkMeta `protobuf:"bytes,1,rep,name=chunks,proto3" json:"chunks,omitempty"`
}

func (x *BlobMeta) Reset() {
	*x = BlobMeta{}
	if protoimpl.UnsafeEnabled {
		mi := &file_protos_blobstore_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BlobMeta) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BlobMeta) ProtoMessage() {}

func (x *BlobMeta) ProtoReflect() protoreflect.Message {
	mi := &file_protos_blobstore_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BlobMeta.ProtoReflect.Descriptor instead.
func (*BlobMeta) Descriptor() ([]byte, []int) {
	return file_protos_blobstore_proto_rawDescGZIP(), []int{0}
}

func (x *BlobMeta) GetChunks() []*BlobMeta_ChunkMeta {
	if x != nil {
		return x.Chunks
	}
	return nil
}

type BlobMeta_ChunkMeta struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Digest []byte `protobuf:"bytes,1,opt,name=digest,proto3" json:"digest,omitempty"`
	Size   uint32 `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
}

func (x *BlobMeta_ChunkMeta) Reset() {
	*x = BlobMeta_ChunkMeta{}
	if protoimpl.UnsafeEnabled {
		mi := &file_protos_blobstore_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BlobMeta_ChunkMeta) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BlobMeta_ChunkMeta) ProtoMessage() {}

func (x *BlobMeta_ChunkMeta) ProtoReflect() protoreflect.Message {
	mi := &file_protos_blobstore_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BlobMeta_ChunkMeta.ProtoReflect.Descriptor instead.
func (*BlobMeta_ChunkMeta) Descriptor() ([]byte, []int) {
	return file_protos_blobstore_proto_rawDescGZIP(), []int{0, 0}
}

func (x *BlobMeta_ChunkMeta) GetDigest() []byte {
	if x != nil {
		return x.Digest
	}
	return nil
}

func (x *BlobMeta_ChunkMeta) GetSize() uint32 {
	if x != nil {
		return x.Size
	}
	return 0
}

var File_protos_blobstore_proto protoreflect.FileDescriptor

var file_protos_blobstore_proto_rawDesc = []byte{
	0x0a, 0x16, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x73, 0x2f, 0x62, 0x6c, 0x6f, 0x62, 0x73, 0x74, 0x6f,
	0x72, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x11, 0x6e, 0x76, 0x69, 0x78, 0x2e, 0x62,
	0x6c, 0x6f, 0x62, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x76, 0x31, 0x22, 0x82, 0x01, 0x0a, 0x08,
	0x42, 0x6c, 0x6f, 0x62, 0x4d, 0x65, 0x74, 0x61, 0x12, 0x3d, 0x0a, 0x06, 0x63, 0x68, 0x75, 0x6e,
	0x6b, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x25, 0x2e, 0x6e, 0x76, 0x69, 0x78, 0x2e,
	0x62, 0x6c, 0x6f, 0x62, 0x73, 0x74, 0x6f, 0x72, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x42, 0x6c, 0x6f,
	0x62, 0x4d, 0x65, 0x74, 0x61, 0x2e, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x4d, 0x65, 0x74, 0x61, 0x52,
	0x06, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x73, 0x1a, 0x37, 0x0a, 0x09, 0x43, 0x68, 0x75, 0x6e, 0x6b,
	0x4d, 0x65, 0x74, 0x61, 0x12, 0x16, 0x0a, 0x06, 0x64, 0x69, 0x67, 0x65, 0x73, 0x74, 0x18, 0x01,
	0x20, 0x01, 0x28, 0x0c, 0x52, 0x06, 0x64, 0x69, 0x67, 0x65, 0x73, 0x74, 0x12, 0x12, 0x0a, 0x04,
	0x73, 0x69, 0x7a, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x04, 0x73, 0x69, 0x7a, 0x65,
	0x42, 0x2a, 0x5a, 0x28, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x62,
	0x72, 0x69, 0x61, 0x6e, 0x6d, 0x63, 0x67, 0x65, 0x65, 0x2f, 0x6e, 0x76, 0x69, 0x78, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x73, 0x3b, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x73, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_protos_blobstore_proto_rawDescOnce sync.Once
	file_protos_blobstore_proto_rawDescData = file_protos_blobstore_proto_rawDesc
)

func file_protos_blobstore_proto_rawDescGZIP() []byte {
	file_protos_blobstore_proto_rawDescOnce.Do(func() {
		file_protos_blobstore_proto_rawDescData = protoimpl.X.CompressGZIP(file_protos_blobstore_proto_rawDescData)
	})
	return file_protos_blobstore_proto_rawDescData
}

var (
	file_protos_blobstore_proto_msgTypes = make([]protoimpl.MessageInfo, 2)
	file_protos_blobstore_proto_goTypes  = []interface{}{
		(*BlobMeta)(nil),           // 0: nvix.blobstore.v1.BlobMeta
		(*BlobMeta_ChunkMeta)(nil), // 1: nvix.blobstore.v1.BlobMeta.ChunkMeta
	}
)
var file_protos_blobstore_proto_depIdxs = []int32{
	1, // 0: nvix.blobstore.v1.BlobMeta.chunks:type_name -> nvix.blobstore.v1.BlobMeta.ChunkMeta
	1, // [1:1] is the sub-list for method output_type
	1, // [1:1] is the sub-list for method input_type
	1, // [1:1] is the sub-list for extension type_name
	1, // [1:1] is the sub-list for extension extendee
	0, // [0:1] is the sub-list for field type_name
}

func init() { file_protos_blobstore_proto_init() }
func file_protos_blobstore_proto_init() {
	if File_protos_blobstore_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_protos_blobstore_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BlobMeta); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_protos_blobstore_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BlobMeta_ChunkMeta); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_protos_blobstore_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   2,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_protos_blobstore_proto_goTypes,
		DependencyIndexes: file_protos_blobstore_proto_depIdxs,
		MessageInfos:      file_protos_blobstore_proto_msgTypes,
	}.Build()
	File_protos_blobstore_proto = out.File
	file_protos_blobstore_proto_rawDesc = nil
	file_protos_blobstore_proto_goTypes = nil
	file_protos_blobstore_proto_depIdxs = nil
}
