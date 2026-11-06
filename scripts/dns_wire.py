"""Check public DNS records through bounded sockets."""

import base64
import datetime
import socket
import struct
import time


def question(name, udp_size=1232):
    labels = name.rstrip(".").split(".")
    wire = b"".join(bytes([len(label)]) + label.encode("ascii") for label in labels) + b"\x00"
    return struct.pack("!6H", 0x1948, 0x100, 1, 0, 0, 1) + wire + struct.pack("!HH", 16, 1) + struct.pack("!BHHIH", 0, 41, udp_size, 0x8000, 0)


def read_exact(connection, length):
    result = bytearray()
    while len(result) < length:
        data = connection.recv(length - len(result))
        if not data:
            raise AssertionError("DNS connection closes before its frame ends")
        result.extend(data)
    return bytes(result)


def query(name, host, port, network, udp_size=1232):
    packet = question(name, udp_size)
    kind = socket.SOCK_DGRAM if network == "udp" else socket.SOCK_STREAM
    with socket.socket(socket.AF_INET, kind) as connection:
        connection.settimeout(2)
        connection.connect((host, port))
        if network == "udp":
            connection.sendall(packet)
            response = connection.recv(65535)
        else:
            connection.sendall(struct.pack("!H", len(packet)) + packet)
            length = struct.unpack("!H", read_exact(connection, 2))[0]
            response = read_exact(connection, length)
    return decode(response)


def skip_name(data, offset):
    for _ in range(128):
        assert offset < len(data), "DNS name exceeds the packet"
        length = data[offset]
        offset += 1
        if length & 0xC0 == 0xC0:
            assert offset < len(data), "DNS pointer exceeds the packet"
            return offset + 1
        if length == 0:
            return offset
        assert length <= 63, "DNS label exceeds 63 bytes"
        offset += length
    raise AssertionError("DNS name exceeds the label bound")


def decode(data):
    identifier, flags, questions, answers, _, _ = struct.unpack("!6H", data[:12])
    assert identifier == 0x1948 and flags & 0x8000, "DNS response changes the request identifier"
    assert flags & 0xF == 0 and flags & 0x400 and not flags & 0xA0, "DNS authority returns invalid flags"
    assert questions == 1, "DNS authority omits the question"
    offset = skip_name(data, 12) + 4
    records = {}
    for _ in range(answers):
        offset = skip_name(data, offset)
        record_type, record_class, _, size = struct.unpack("!HHIH", data[offset:offset + 10])
        assert record_class == 1, "DNS response changes the record class"
        offset += 10
        end = offset + size
        assert end <= len(data), "DNS record exceeds the packet"
        if record_type == 16:
            chunks = []
            cursor = offset
            while cursor < end:
                length = data[cursor]
                cursor += 1
                chunks.append(data[cursor:cursor + length])
                cursor += length
            assert cursor == end, "TXT string exceeds its record"
            records["content"] = b"".join(chunks).decode("utf-8")
        elif record_type == 46:
            covered, algorithm, _, _, expiration, inception, _ = struct.unpack("!HBBIIIH", data[offset:offset + 18])
            assert covered == 16 and algorithm == 13, "DNS response changes the TXT signature algorithm"
            records.update(expiration=expiration, inception=inception, signature=data[skip_name(data, offset + 18):end])
        offset = end
    records["truncated"] = bool(flags & 0x200)
    records["answers"] = answers
    return records


def assert_record(record, response):
    assert not response["truncated"] and response["answers"] == 2, "DNS response omits the signed TXT RRset"
    assert response["content"] == record["content"], "DNS changes the public content"
    assert response["signature"] == base64.b64decode(record["rrsig"].split()[-1], validate=True), "DNS changes the stored signature"
    for field in ("inception", "expiration"):
        expected = int(datetime.datetime.fromisoformat(record[field].replace("Z", "+00:00")).timestamp())
        assert response[field] == expected, "DNS changes the explicit signature boundary"
    assert response["inception"] > time.time(), "The synthetic signature is no longer in the future"


def assert_authority(record, host="127.0.0.1", port=1053):
    for network in ("udp", "tcp"):
        assert_record(record, query(record["name"], host, port, network))


def assert_fallback(record, host="127.0.0.1", port=1053):
    response = query(record["name"], host, port, "udp", 512)
    assert response["truncated"] and response["answers"] == 0, "UDP does not require TCP fallback"
    assert_record(record, query(record["name"], host, port, "tcp", 512))
