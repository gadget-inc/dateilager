package key

import (
	"github.com/gadget-inc/dateilager/pkg/stringutil"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap"
)

const (
	Count             = Int64Key("dl.count")
	DiffCount         = Uint32Key("dl.diff_count")
	Directory         = StringKey("dl.directory")
	Environment       = StringKey("dl.environment")
	FromVersion       = Int64pKey("dl.from_version")
	KeepVersions      = Int64Key("dl.keep_versions")
	LatestVersion     = Int64Key("dl.latest_version")
	LiveObjectsCount  = Int64Key("dl.live_objects_count")
	ObjectPath        = StringKey("dl.object.path")
	ObjectsCount      = IntKey("dl.object_count")
	ObjectsParent     = StringKey("dl.object_parent")
	PackPatterns      = StringSliceKey("dl.pack_patterns")
	Port              = IntKey("dl.port")
	Prefix            = StringKey("dl.prefix")
	Project           = Int64Key("dl.project")
	QueryIgnores      = StringSliceKey("dl.query.ignores")
	QueryIsPrefix     = BoolKey("dl.query.is_prefix")
	QueryPath         = StringKey("dl.query.path")
	QueryWithContent  = BoolKey("dl.query.with_content")
	SampleRate        = Float32Key("dl.sample_rate")
	Server            = StringKey("dl.server")
	State             = StringKey("dl.state")
	Template          = Int64pKey("dl.template")
	ToVersion         = Int64pKey("dl.to_version")
	TotalObjectsCount = Int64Key("dl.total_objects_count")
	Version           = Int64Key("dl.version")
	Worker            = IntKey("dl.worker")
	Ignores           = StringSliceKey("dl.ignores")
)

var (
	ObjectContent = ShortenedStringKey{"dl.object.content", 10}
)

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
