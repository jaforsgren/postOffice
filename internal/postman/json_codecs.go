package postman

// Each Postman type keeps the keys it does not model; see unknownFields.

func (c *Collection) UnmarshalJSON(data []byte) error {
	type plain Collection
	return decodeKeepingUnknown(data, (*plain)(c), &c.unknown)
}

func (c Collection) MarshalJSON() ([]byte, error) {
	type plain Collection
	return encodeWithUnknown(plain(c), c.unknown)
}

func (i *Info) UnmarshalJSON(data []byte) error {
	type plain Info
	return decodeKeepingUnknown(data, (*plain)(i), &i.unknown)
}

func (i Info) MarshalJSON() ([]byte, error) {
	type plain Info
	return encodeWithUnknown(plain(i), i.unknown)
}

func (i *Item) UnmarshalJSON(data []byte) error {
	type plain Item
	return decodeKeepingUnknown(data, (*plain)(i), &i.unknown)
}

func (i Item) MarshalJSON() ([]byte, error) {
	type plain Item
	return encodeWithUnknown(plain(i), i.unknown)
}

func (r *Request) UnmarshalJSON(data []byte) error {
	type plain Request
	return decodeKeepingUnknown(data, (*plain)(r), &r.unknown)
}

func (r Request) MarshalJSON() ([]byte, error) {
	type plain Request
	return encodeWithUnknown(plain(r), r.unknown)
}

func (h *Header) UnmarshalJSON(data []byte) error {
	type plain Header
	return decodeKeepingUnknown(data, (*plain)(h), &h.unknown)
}

func (h Header) MarshalJSON() ([]byte, error) {
	type plain Header
	return encodeWithUnknown(plain(h), h.unknown)
}

func (b *Body) UnmarshalJSON(data []byte) error {
	type plain Body
	return decodeKeepingUnknown(data, (*plain)(b), &b.unknown)
}

func (b Body) MarshalJSON() ([]byte, error) {
	type plain Body
	return encodeWithUnknown(plain(b), b.unknown)
}

func (f *FormParam) UnmarshalJSON(data []byte) error {
	type plain FormParam
	return decodeKeepingUnknown(data, (*plain)(f), &f.unknown)
}

func (f FormParam) MarshalJSON() ([]byte, error) {
	type plain FormParam
	return encodeWithUnknown(plain(f), f.unknown)
}

func (u *URL) UnmarshalJSON(data []byte) error {
	type plain URL
	return decodeKeepingUnknown(data, (*plain)(u), &u.unknown)
}

func (u URL) MarshalJSON() ([]byte, error) {
	type plain URL
	return encodeWithUnknown(plain(u), u.unknown)
}

func (e *Environment) UnmarshalJSON(data []byte) error {
	type plain Environment
	return decodeKeepingUnknown(data, (*plain)(e), &e.unknown)
}

func (e Environment) MarshalJSON() ([]byte, error) {
	type plain Environment
	return encodeWithUnknown(plain(e), e.unknown)
}

func (e *EnvVariable) UnmarshalJSON(data []byte) error {
	type plain EnvVariable
	return decodeKeepingUnknown(data, (*plain)(e), &e.unknown)
}

func (e EnvVariable) MarshalJSON() ([]byte, error) {
	type plain EnvVariable
	return encodeWithUnknown(plain(e), e.unknown)
}

func (v *Variable) UnmarshalJSON(data []byte) error {
	type plain Variable
	return decodeKeepingUnknown(data, (*plain)(v), &v.unknown)
}

func (v Variable) MarshalJSON() ([]byte, error) {
	type plain Variable
	return encodeWithUnknown(plain(v), v.unknown)
}

func (e *Event) UnmarshalJSON(data []byte) error {
	type plain Event
	return decodeKeepingUnknown(data, (*plain)(e), &e.unknown)
}

func (e Event) MarshalJSON() ([]byte, error) {
	type plain Event
	return encodeWithUnknown(plain(e), e.unknown)
}

func (s *Script) UnmarshalJSON(data []byte) error {
	type plain Script
	return decodeKeepingUnknown(data, (*plain)(s), &s.unknown)
}

func (s Script) MarshalJSON() ([]byte, error) {
	type plain Script
	return encodeWithUnknown(plain(s), s.unknown)
}
