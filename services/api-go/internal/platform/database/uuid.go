package database

import (
	"database/sql/driver"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
)

type googleUUIDCodec struct{}

func registerGoogleUUID(m *pgtype.Map) {
	m.RegisterType(&pgtype.Type{Name: "uuid", OID: pgtype.UUIDOID, Codec: googleUUIDCodec{}})
	m.RegisterDefaultPgType(uuid.UUID{}, "uuid")
	var pointer *uuid.UUID
	m.RegisterDefaultPgType(pointer, "uuid")
}

func (googleUUIDCodec) FormatSupported(format int16) bool {
	return format == pgtype.TextFormatCode || format == pgtype.BinaryFormatCode
}

func (googleUUIDCodec) PreferredFormat() int16 { return pgtype.TextFormatCode }

func (googleUUIDCodec) PlanEncode(m *pgtype.Map, oid uint32, format int16, value any) pgtype.EncodePlan {
	switch value.(type) {
	case uuid.UUID, *uuid.UUID:
		return encodeGoogleUUID{format: format}
	default:
		return (pgtype.UUIDCodec{}).PlanEncode(m, oid, format, value)
	}
}

func (googleUUIDCodec) PlanScan(m *pgtype.Map, oid uint32, format int16, target any) pgtype.ScanPlan {
	switch target.(type) {
	case *uuid.UUID:
		return scanGoogleUUID{format: format}
	case **uuid.UUID:
		return scanGoogleUUIDPointer{format: format}
	default:
		return (pgtype.UUIDCodec{}).PlanScan(m, oid, format, target)
	}
}

func (c googleUUIDCodec) DecodeDatabaseSQLValue(m *pgtype.Map, oid uint32, format int16, src []byte) (driver.Value, error) {
	return c.DecodeValue(m, oid, format, src)
}

func (googleUUIDCodec) DecodeValue(_ *pgtype.Map, _ uint32, format int16, src []byte) (any, error) {
	if src == nil {
		return nil, nil
	}
	id, err := parseGoogleUUID(src, format)
	if err != nil {
		return nil, err
	}
	return id.String(), nil
}

type encodeGoogleUUID struct{ format int16 }

func (e encodeGoogleUUID) Encode(value any, buf []byte) ([]byte, error) {
	switch typed := value.(type) {
	case uuid.UUID:
		return appendUUID(buf, typed, e.format), nil
	case *uuid.UUID:
		if typed == nil {
			return nil, nil
		}
		return appendUUID(buf, *typed, e.format), nil
	default:
		return nil, fmt.Errorf("cannot encode %T as uuid", value)
	}
}

type scanGoogleUUID struct{ format int16 }

func (s scanGoogleUUID) Scan(src []byte, dst any) error {
	target := dst.(*uuid.UUID)
	if src == nil {
		return fmt.Errorf("cannot scan NULL into uuid.UUID")
	}
	id, err := parseGoogleUUID(src, s.format)
	if err != nil {
		return err
	}
	*target = id
	return nil
}

type scanGoogleUUIDPointer struct{ format int16 }

func (s scanGoogleUUIDPointer) Scan(src []byte, dst any) error {
	target := dst.(**uuid.UUID)
	if src == nil {
		*target = nil
		return nil
	}
	id, err := parseGoogleUUID(src, s.format)
	if err != nil {
		return err
	}
	*target = &id
	return nil
}

func appendUUID(buf []byte, id uuid.UUID, format int16) []byte {
	if format == pgtype.BinaryFormatCode {
		return append(buf, id[:]...)
	}
	return append(buf, id.String()...)
}

func parseGoogleUUID(src []byte, format int16) (uuid.UUID, error) {
	if format == pgtype.BinaryFormatCode {
		if len(src) != 16 {
			return uuid.Nil, fmt.Errorf("uuid binary length %d", len(src))
		}
		return uuid.FromBytes(src)
	}
	return uuid.Parse(string(src))
}
