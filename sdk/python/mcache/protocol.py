"""mcache binary wire protocol encode / decode.

Frame layout (10-byte header + payload):
    4 bytes: payload length (big-endian uint32)
    4 bytes: stream ID   (big-endian uint32)
    1 byte:  type        (0=request, 1=response)
    1 byte:  flags
    N bytes: payload
"""

import struct

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

FRAME_TYPE_REQUEST = 0
FRAME_TYPE_RESPONSE = 1

STATUS_OK = 0
STATUS_ERR = 1
STATUS_NOT_FOUND = 2

MAX_PAYLOAD_SIZE = 16 * 1024 * 1024  # 16 MB

# --- Command codes ---

# KV (1-5)
CMD_GET, CMD_SET, CMD_DEL, CMD_LEN, CMD_CLEANUP = 1, 2, 3, 4, 5

# Set (10-19)
CMD_SADD, CMD_SREM, CMD_SISMEMBER, CMD_SMEMBERS, CMD_SCARD = 10, 11, 12, 13, 14
CMD_SPOP, CMD_SRANDMEMBER, CMD_SUNION, CMD_SINTER, CMD_SDIFF = 15, 16, 17, 18, 19

# Hash (32-45)
CMD_HSET, CMD_HGET, CMD_HDEL, CMD_HEXISTS, CMD_HGETALL = 32, 33, 34, 35, 36
CMD_HKEYS, CMD_HVALS, CMD_HLEN, CMD_HSTRLEN = 37, 38, 39, 40
CMD_HINCRBY, CMD_HINCRBYFLOAT, CMD_HMGET, CMD_HMSET, CMD_HSETNX = 41, 42, 43, 44, 45

# List (48-61)
CMD_LPUSH, CMD_RPUSH, CMD_LPOP, CMD_RPOP, CMD_LLEN = 48, 49, 50, 51, 52
CMD_LRANGE, CMD_LINDEX, CMD_LSET, CMD_LREM = 53, 54, 55, 56
CMD_LTRIM, CMD_LINSERT, CMD_BLPOP, CMD_BRPOP, CMD_LPOS = 57, 58, 59, 60, 61

# Key management (64-73)
CMD_EXISTS, CMD_TYPE = 64, 65
CMD_EXPIRE, CMD_EXPIREAT, CMD_PEXPIRE, CMD_PEXPIREAT = 66, 67, 68, 69
CMD_TTL, CMD_PTTL, CMD_PERSIST, CMD_KEYS = 70, 71, 72, 73

# Key type constants
KEYTYPE_NONE, KEYTYPE_STRING, KEYTYPE_SET, KEYTYPE_HASH, KEYTYPE_LIST = 0, 1, 2, 3, 4

_SET_CMDS = frozenset(range(10, 20))
_HASH_CMDS = frozenset(range(32, 46))
_LIST_CMDS = frozenset(range(48, 62))
_KEY_CMDS = frozenset(range(64, 74))


def _is_set(cmd): return 10 <= cmd <= 19
def _is_hash(cmd): return 32 <= cmd <= 45
def _is_list(cmd): return 48 <= cmd <= 61
def _is_key(cmd): return 64 <= cmd <= 73


# ---------------------------------------------------------------------------
# Frame helpers
# ---------------------------------------------------------------------------

_HEADER_FMT = '>IIBB'  # payload_len, stream_id, type, flags
_HEADER_SIZE = struct.calcsize(_HEADER_FMT)  # 10

_FRAME_HEADER = struct.Struct(_HEADER_FMT)


def encode_frame(stream_id: int, payload: bytes, flags: int = 0) -> bytes:
    """Encode a request frame (type=0)."""
    return _FRAME_HEADER.pack(len(payload), stream_id, FRAME_TYPE_REQUEST, flags) + payload


def decode_frame_header(data: bytes) -> tuple:
    """Decode the 10-byte frame header. Returns (payload_len, stream_id, ftype, flags)."""
    return _FRAME_HEADER.unpack(data)


# ---------------------------------------------------------------------------
# KV request / response
# ---------------------------------------------------------------------------

def encode_kv_request(cmd: int, key: str, value: bytes = b'', ttl_ms: int = 0) -> bytes:
    """Encode a KV request payload.
    Layout: [1B cmd][2B key_len][4B val_len][8B ttl_ms][key][value]
    """
    kb = key.encode()
    payload = struct.pack('>BHIq', cmd, len(kb), len(value), ttl_ms) + kb + value
    return payload


def decode_kv_response(payload: bytes) -> tuple:
    """Decode a KV response payload.
    Returns (status: int, value: bytes, err_msg: str).
    """
    if len(payload) < 7:
        raise ValueError('KV response payload too short')
    status, val_len, err_len = struct.unpack_from('>BIH', payload, 0)
    offset = 7
    value = b''
    err_msg = ''
    if val_len > 0 and offset + val_len <= len(payload):
        value = payload[offset:offset + val_len]
        offset += val_len
    if err_len > 0 and offset + err_len <= len(payload):
        err_msg = payload[offset:offset + err_len].decode()
    return status, value, err_msg


# ---------------------------------------------------------------------------
# Set request / response
# ---------------------------------------------------------------------------

def encode_set_request(cmd: int, key: str, elems: list = None,
                       count: int = 0, keys: list = None) -> bytes:
    """Encode a Set request payload."""
    kb = key.encode()
    if cmd in (CMD_SMEMBERS, CMD_SCARD, CMD_SPOP):
        return struct.pack('>BH', cmd, len(kb)) + kb
    elif cmd in (CMD_SADD, CMD_SREM):
        if elems is None:
            elems = []
        parts = [struct.pack('>BHI', cmd, len(kb), len(elems)) + kb]
        for e in elems:
            eb = e.encode()
            parts.append(struct.pack('>I', len(eb)) + eb)
        return b''.join(parts)
    elif cmd == CMD_SISMEMBER:
        e = elems[0] if elems else ''
        eb = e.encode()
        return struct.pack('>BHI', cmd, len(kb), len(eb)) + kb + eb
    elif cmd == CMD_SRANDMEMBER:
        return struct.pack('>BHI', cmd, len(kb), count) + kb
    elif cmd in (CMD_SUNION, CMD_SINTER, CMD_SDIFF):
        if keys is None:
            keys = []
        parts = [struct.pack('>BH', cmd, len(keys))]
        for k in keys:
            ek = k.encode()
            parts.append(struct.pack('>H', len(ek)) + ek)
        return b''.join(parts)
    raise ValueError(f'unknown set command: {cmd}')


def decode_set_response(payload: bytes, cmd: int) -> dict:
    """Decode a Set response. Returns dict with command-specific fields."""
    if not payload:
        raise ValueError('empty set response')
    status = payload[0]
    if status == STATUS_ERR:
        return {'status': status, 'err_msg': payload[1:].decode() if len(payload) > 1 else ''}
    result = {'status': status}
    if cmd in (CMD_SADD, CMD_SREM):
        result['changed'] = struct.unpack_from('>Q', payload, 1)[0]
    elif cmd == CMD_SISMEMBER:
        result['is_member'] = payload[1] != 0 if len(payload) > 1 else False
    elif cmd == CMD_SCARD:
        result['card'] = struct.unpack_from('>Q', payload, 1)[0]
    elif cmd == CMD_SPOP:
        if len(payload) >= 3:
            elen = struct.unpack_from('>H', payload, 1)[0]
            if elen > 0 and 3 + elen <= len(payload):
                result['elem'] = payload[3:3 + elen].decode()
            else:
                result['elem'] = None
        else:
            result['elem'] = None
    elif cmd in (CMD_SMEMBERS, CMD_SRANDMEMBER, CMD_SUNION, CMD_SINTER, CMD_SDIFF):
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            elems = []
            off = 5
            for _ in range(count):
                if off + 2 > len(payload):
                    break
                elen = struct.unpack_from('>H', payload, off)[0]
                off += 2
                if off + elen > len(payload):
                    break
                elems.append(payload[off:off + elen].decode())
                off += elen
            result['elems'] = elems
        else:
            result['elems'] = []
    return result


# ---------------------------------------------------------------------------
# Hash request / response
# ---------------------------------------------------------------------------

def encode_hash_request(cmd: int, key: str, field: str = '', value: str = '',
                        fields: list = None, fv_pairs: list = None,
                        delta_i64: int = 0, delta_f64: float = 0.0) -> bytes:
    """Encode a Hash request payload."""
    kb = key.encode()
    if cmd in (CMD_HGETALL, CMD_HKEYS, CMD_HVALS, CMD_HLEN):
        return struct.pack('>BH', cmd, len(kb)) + kb
    elif cmd in (CMD_HGET, CMD_HEXISTS, CMD_HSTRLEN):
        fb = field.encode()
        return struct.pack('>BHH', cmd, len(kb), len(fb)) + kb + fb
    elif cmd in (CMD_HSET, CMD_HSETNX):
        fb, vb = field.encode(), value.encode()
        return struct.pack('>BHHI', cmd, len(kb), len(fb), len(vb)) + kb + fb + vb
    elif cmd in (CMD_HDEL, CMD_HMGET):
        if fields is None:
            fields = []
        parts = [struct.pack('>BHH', cmd, len(kb), len(fields)), kb]
        for f in fields:
            fb = f.encode()
            parts.append(struct.pack('>H', len(fb)) + fb)
        return b''.join(parts)
    elif cmd == CMD_HINCRBY:
        fb = field.encode()
        return struct.pack('>BHHq', cmd, len(kb), len(fb), delta_i64) + kb + fb
    elif cmd == CMD_HINCRBYFLOAT:
        import struct as _s
        fb = field.encode()
        bits = _s.pack('>d', delta_f64)
        return struct.pack('>BHH', cmd, len(kb), len(fb)) + bits + kb + fb
    elif cmd == CMD_HMSET:
        if fv_pairs is None:
            fv_pairs = []
        parts = [struct.pack('>BHH', cmd, len(kb), len(fv_pairs) // 2), kb]
        for i in range(0, len(fv_pairs) - 1, 2):
            fb = fv_pairs[i].encode()
            vb = fv_pairs[i + 1].encode()
            parts.append(struct.pack('>H', len(fb)) + fb)
            parts.append(struct.pack('>H', len(vb)) + vb)
        return b''.join(parts)
    raise ValueError(f'unknown hash command: {cmd}')


def decode_hash_response(payload: bytes, cmd: int) -> dict:
    """Decode a Hash response."""
    if not payload:
        raise ValueError('empty hash response')
    status = payload[0]
    if status == STATUS_ERR:
        err_len = struct.unpack_from('>H', payload, 1)[0] if len(payload) >= 3 else 0
        return {'status': status, 'err_msg': payload[3:3 + err_len].decode() if err_len else ''}
    result = {'status': status}
    if cmd in (CMD_HSET, CMD_HDEL, CMD_HLEN, CMD_HSTRLEN, CMD_HINCRBY):
        result['int_result'] = struct.unpack_from('>q', payload, 1)[0]
    elif cmd in (CMD_HEXISTS, CMD_HSETNX):
        result['bool_result'] = payload[1] != 0 if len(payload) > 1 else False
    elif cmd == CMD_HGET:
        if len(payload) >= 5:
            vlen = struct.unpack_from('>I', payload, 1)[0]
            result['str_result'] = payload[5:5 + vlen].decode() if vlen else ''
        else:
            result['str_result'] = ''
    elif cmd == CMD_HGETALL:
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            off = 5
            map_result = {}
            for _ in range(count):
                if off + 2 > len(payload):
                    break
                klen = struct.unpack_from('>H', payload, off)[0]; off += 2
                if off + klen > len(payload):
                    break
                k = payload[off:off + klen].decode(); off += klen
                if off + 4 > len(payload):
                    break
                vlen = struct.unpack_from('>I', payload, off)[0]; off += 4
                if off + vlen > len(payload):
                    break
                v = payload[off:off + vlen].decode(); off += vlen
                map_result[k] = v
            result['map_result'] = map_result
        else:
            result['map_result'] = {}
    elif cmd in (CMD_HKEYS, CMD_HVALS):
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            off = 5
            elems = []
            for _ in range(count):
                if off + 2 > len(payload):
                    break
                elen = struct.unpack_from('>H', payload, off)[0]; off += 2
                if off + elen > len(payload):
                    break
                elems.append(payload[off:off + elen].decode()); off += elen
            result['slice_result'] = elems
        else:
            result['slice_result'] = []
    elif cmd == CMD_HINCRBYFLOAT:
        if len(payload) >= 9:
            import struct as _s
            bits = struct.unpack_from('>Q', payload, 1)[0]
            result['float_result'] = _s.unpack('>d', _s.pack('>Q', bits))[0]
        else:
            result['float_result'] = 0.0
    elif cmd == CMD_HMGET:
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            off = 5
            any_slice = []
            for _ in range(count):
                if off >= len(payload):
                    break
                has_val = payload[off]; off += 1
                if has_val:
                    if off + 4 > len(payload):
                        break
                    vlen = struct.unpack_from('>I', payload, off)[0]; off += 4
                    if off + vlen > len(payload):
                        break
                    any_slice.append(payload[off:off + vlen].decode()); off += vlen
                else:
                    any_slice.append(None)
            result['any_slice'] = any_slice
        else:
            result['any_slice'] = []
    return result


# ---------------------------------------------------------------------------
# List request / response
# ---------------------------------------------------------------------------

def encode_list_request(cmd: int, key: str, elements: list = None, value: str = '',
                        index: int = 0, start: int = 0, stop: int = 0,
                        count: int = 0, pivot: str = '', before: bool = False,
                        timeout: int = 0, rank: int = 0, max_len: int = 0) -> bytes:
    """Encode a List request payload."""
    kb = key.encode()
    if cmd in (CMD_LPOP, CMD_RPOP, CMD_LLEN):
        return struct.pack('>BH', cmd, len(kb)) + kb
    elif cmd in (CMD_LPUSH, CMD_RPUSH):
        if elements is None:
            elements = []
        parts = [struct.pack('>BHI', cmd, len(kb), len(elements)), kb]
        for e in elements:
            eb = e.encode()
            parts.append(struct.pack('>H', len(eb)) + eb)
        return b''.join(parts)
    elif cmd in (CMD_LRANGE, CMD_LTRIM):
        return struct.pack('>BHqq', cmd, len(kb), start, stop) + kb
    elif cmd == CMD_LINDEX:
        return struct.pack('>BHq', cmd, len(kb), index) + kb
    elif cmd == CMD_LSET:
        vb = value.encode()
        return struct.pack('>BHqI', cmd, len(kb), index, len(vb)) + kb + vb
    elif cmd == CMD_LREM:
        vb = value.encode()
        return struct.pack('>BHqI', cmd, len(kb), count, len(vb)) + kb + vb
    elif cmd == CMD_LINSERT:
        pb = pivot.encode()
        vb = value.encode()
        before_byte = 1 if before else 0
        return (struct.pack('>BH', cmd, len(kb)) + bytes([before_byte]) +
                struct.pack('>HI', len(pb), len(vb)) + kb + pb + vb)
    elif cmd in (CMD_BLPOP, CMD_BRPOP):
        return struct.pack('>BHq', cmd, len(kb), timeout) + kb
    elif cmd == CMD_LPOS:
        vb = value.encode()
        return struct.pack('>BHqqqI', cmd, len(kb), rank, count, max_len, len(vb)) + kb + vb
    raise ValueError(f'unknown list command: {cmd}')


def decode_list_response(payload: bytes, cmd: int) -> dict:
    """Decode a List response."""
    if not payload:
        raise ValueError('empty list response')
    status = payload[0]
    if status == STATUS_ERR:
        err_len = struct.unpack_from('>H', payload, 1)[0] if len(payload) >= 3 else 0
        return {'status': status, 'err_msg': payload[3:3 + err_len].decode() if err_len else ''}
    result = {'status': status}
    if cmd in (CMD_LPUSH, CMD_RPUSH, CMD_LLEN, CMD_LREM, CMD_LINSERT):
        result['int_result'] = struct.unpack_from('>q', payload, 1)[0]
    elif cmd in (CMD_LPOP, CMD_RPOP, CMD_BLPOP, CMD_BRPOP, CMD_LINDEX):
        if len(payload) >= 5:
            vlen = struct.unpack_from('>I', payload, 1)[0]
            result['str_result'] = payload[5:5 + vlen].decode() if vlen else ''
        else:
            result['str_result'] = ''
    elif cmd == CMD_LRANGE:
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            off = 5
            elems = []
            for _ in range(count):
                if off + 2 > len(payload):
                    break
                elen = struct.unpack_from('>H', payload, off)[0]; off += 2
                if off + elen > len(payload):
                    break
                elems.append(payload[off:off + elen].decode()); off += elen
            result['slice_result'] = elems
        else:
            result['slice_result'] = []
    elif cmd == CMD_LSET:
        result['bool_result'] = payload[1] != 0 if len(payload) > 1 else False
    elif cmd == CMD_LPOS:
        if len(payload) >= 5:
            count = struct.unpack_from('>I', payload, 1)[0]
            off = 5
            pos_result = []
            for _ in range(count):
                if off + 8 > len(payload):
                    break
                pos_result.append(struct.unpack_from('>q', payload, off)[0])
                off += 8
            result['pos_result'] = pos_result
        else:
            result['pos_result'] = []
    return result


# ---------------------------------------------------------------------------
# Key management (reuses KV request/response format)
# ---------------------------------------------------------------------------

def encode_key_request(cmd: int, key: str, extra: int = 0) -> bytes:
    """Encode a key-management request. 'extra' holds seconds/ms for EXPIRE etc."""
    kb = key.encode()
    if cmd == CMD_EXPIRE:
        return struct.pack('>BHq', cmd, len(kb), extra * 1000) + kb
    elif cmd == CMD_PEXPIRE:
        return struct.pack('>BHq', cmd, len(kb), extra) + kb
    elif cmd == CMD_EXPIREAT:
        return struct.pack('>BHq', cmd, len(kb), extra * 1000) + kb
    elif cmd == CMD_PEXPIREAT:
        return struct.pack('>BHq', cmd, len(kb), extra) + kb
    else:
        # EXISTS, TYPE, TTL, PTTL, PERSIST, KEYS
        return struct.pack('>BHq', cmd, len(kb), 0) + kb
