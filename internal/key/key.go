package key

import (
	"time"

	"github.com/gadget-inc/dateilager/pkg/stringutil"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	Args               = StringSliceKey("dl.args")
	CacheDir           = StringKey("dl.cache_dir")
	CachePath          = StringKey("dl.cache_path")
	CacheVersion       = Int64Key("dl.cache_version")
	CacheVersions      = Int64SliceKey("dl.cache_versions")
	CachedCount        = Uint32Key("dl.cached_count")
	CloneToProject     = Int64Key("dl.clone_to_project")
	Command            = StringKey("dl.command")
	Count              = Int64Key("dl.count")
	Device             = StringKey("dl.device")
	DeviceGlob         = StringKey("dl.device_glob")
	DiffCount          = Uint32Key("dl.diff_count")
	Directory          = StringKey("dl.directory")
	DurationMS         = DurationKey("dl.duration_ms")
	Environment        = StringKey("dl.environment")
	FileMatch          = BoolKey("dl.file_match")
	FromVersion        = Int64pKey("dl.from_version")
	HasReflinkSupport  = BoolKey("dl.has_reflink_support")
	Ignores            = StringSliceKey("dl.ignores")
	KeepVersions       = Int64Key("dl.keep_versions")
	LatestVersion      = Int64Key("dl.latest_version")
	LiveObjectsCount   = Int64Key("dl.live_objects_count")
	LogicalVolume      = StringKey("dl.logical_volume")
	MaxContentSendSize = Int64Key("dl.max_content_send_size")
	ObjectPath         = StringKey("dl.object.path")
	ObjectsCount       = IntKey("dl.object_count")
	ObjectsParent      = StringKey("dl.object_parent")
	PackPatterns       = StringSliceKey("dl.pack_patterns")
	Path               = StringKey("dl.path")
	Port               = IntKey("dl.port")
	Prefix             = StringKey("dl.prefix")
	Project            = Int64Key("dl.project")
	QueryIgnores       = StringSliceKey("dl.query.ignores")
	QueryIsPrefix      = BoolKey("dl.query.is_prefix")
	QueryPath          = StringKey("dl.query.path")
	SampleRate         = Float32Key("dl.sample_rate")
	Server             = StringKey("dl.server")
	Socket             = StringKey("dl.socket")
	State              = StringKey("dl.state")
	Subpaths           = StringSliceKey("dl.subpaths")
	Summarize          = BoolKey("dl.summarize")
	TargetPath         = StringKey("dl.target_path")
	Template           = Int64pKey("dl.template")
	ThinPool           = StringKey("dl.thin_pool")
	ToVersion          = Int64pKey("dl.to_version")
	TotalObjectsCount  = Int64Key("dl.total_objects_count")
	UpperDir           = StringKey("dl.upper_dir")
	Version            = Int64Key("dl.version")
	VirtualSize        = StringKey("dl.virtual_size")
	VolumeGroup        = StringKey("dl.volume_group")
	VolumeID           = StringKey("dl.volume_id")
	VolumePath         = StringKey("dl.volume_path")
	WorkDir            = StringKey("dl.work_dir")
	Worker             = IntKey("dl.worker")
	WorkerCount        = IntKey("dl.worker_count")
)

var ObjectContent = ShortenedStringKey{"dl.object.content", 10}

type BoolKey string

func (bk BoolKey) Field(value bool) zap.Field {
	return zap.Bool(string(bk), value)
}

func (bk BoolKey) Attribute(value bool) attribute.KeyValue {
	return attribute.Bool(string(bk), value)
}

type StringKey string

func (sk StringKey) Field(value string) zap.Field {
	return zap.String(string(sk), value)
}

func (sk StringKey) Attribute(value string) attribute.KeyValue {
	return attribute.String(string(sk), value)
}

type StringSliceKey string

func (ssk StringSliceKey) Field(value []string) zap.Field {
	return zap.Strings(string(ssk), value)
}

func (ssk StringSliceKey) Attribute(value []string) attribute.KeyValue {
	return attribute.StringSlice(string(ssk), value)
}

type ShortenedStringKey struct {
	key string
	n   int
}

func (s ShortenedStringKey) Field(value string) zap.Field {
	return zap.String(s.key, stringutil.ShortenString(value, s.n))
}

func (s ShortenedStringKey) Attribute(value string) attribute.KeyValue {
	return attribute.String(s.key, stringutil.ShortenString(value, s.n))
}

type IntKey string

func (ik IntKey) Field(value int) zap.Field {
	return zap.Int(string(ik), value)
}

func (ik IntKey) Attribute(value int) attribute.KeyValue {
	return attribute.Int(string(ik), value)
}

type Int64Key string

func (ik Int64Key) Field(value int64) zap.Field {
	return zap.Int64(string(ik), value)
}

func (ik Int64Key) Attribute(value int64) attribute.KeyValue {
	return attribute.Int64(string(ik), value)
}

type Int64pKey string

func (ik Int64pKey) Field(value *int64) zap.Field {
	return zap.Int64p(string(ik), value)
}

func (ik Int64pKey) Attribute(value *int64) attribute.KeyValue {
	if value == nil {
		return attribute.String(string(ik), "")
	}
	return attribute.Int64(string(ik), *value)
}

type Uint32Key string

func (uk Uint32Key) Field(value uint32) zap.Field {
	return zap.Uint32(string(uk), value)
}

func (uk Uint32Key) Attribute(value uint32) attribute.KeyValue {
	return attribute.Int(string(uk), int(value))
}

type Float32Key string

func (fk Float32Key) Field(value float32) zap.Field {
	return zap.Float32(string(fk), value)
}

func (fk Float32Key) Attribute(value float32) attribute.KeyValue {
	return attribute.Float64(string(fk), float64(value))
}

type Int64SliceKey string

func (isk Int64SliceKey) Field(value []int64) zap.Field {
	return zap.Int64s(string(isk), value)
}

func (isk Int64SliceKey) Attribute(value []int64) attribute.KeyValue {
	return attribute.Int64Slice(string(isk), value)
}

type DurationKey string

func (dk DurationKey) Field(value time.Duration) zap.Field {
	return zap.Float64(string(dk), float64(value.Milliseconds()))
}

func (dk DurationKey) Attribute(value time.Duration) attribute.KeyValue {
	return attribute.Float64(string(dk), float64(value.Milliseconds()))
}
