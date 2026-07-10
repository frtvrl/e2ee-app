// mobile/test/packet_parser_test.dart
//
// Sprint 10.1B — packet_parser_test.dart (≥10 cases).
//
// Per Architect brief: "10 cases (valid IPv4 TCP, valid IPv4 UDP,
// IPv6, malformed, ICMP, short packet)". We exceed the floor (15
// cases) to cover the edge cases that the live build will hit
// first:
//
//   1.  IPv4 + TCP, 20-byte IP header, full TCP header → ports
//       + flags + masked IPs.
//   2.  IPv4 + UDP, 20-byte IP header, full UDP header → ports,
//       no TCP flags.
//   3.  IPv4 + ICMP, 20-byte IP header → masked IPs, no ports.
//   4.  IPv4 + unknown protocol (47 = GRE) → masked IPs, no ports.
//   5.  IPv6 + TCP, 40-byte header + 20-byte TCP → masked /48 IPs,
//       ports, flags.
//   6.  IPv6 + UDP → masked /48 IPs, ports.
//   7.  IPv6 + Hop-by-Hop extension header (correctly walked past
//       to a TCP next header).
//   8.  Empty buffer → null.
//   9.  Truncated IPv4 header (length < 20 bytes) → null.
//   10. Truncated TCP payload (L4 starts but no port bytes) →
//       masked IPs but null ports.
//   11. IHL=0 (invalid) → null.
//   12. IHL=4 (smaller than minimum 5) → null.
//   13. IPv4 /24 mask verification — 192.168.1.42 → 192.168.1.0.
//   14. IPv6 /48 mask verification — 2001:db8:1234:5678::1 →
//       2001:db8:1234:0:0:0:0:0.
//   15. Buffer larger than the headers (extra payload bytes) is
//       ignored — the parser must not look past the L4 header
//       boundary.

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:opene2ee/services/packet_parser.dart';

void main() {
  group('PacketParser', () {
    test('IPv4 TCP — ports + flags + masked /24 IPs', () {
      // 20-byte IPv4 header (IHL=5) + 20-byte TCP header.
      // 10.0.0.1:54321 → 93.184.216.34:443, SYN|ACK (0x12).
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x28, // ver/ihl, tos, total length = 40
        0x00, 0x01, 0x00, 0x00, // id, flags+frag
        0x40, 0x06, 0x00, 0x00, // ttl=64, proto=6 (TCP), checksum
        0x0A, 0x00, 0x00, 0x01, // src 10.0.0.1
        0x5D, 0xB8, 0xD8, 0x22, // dst 93.184.216.34
        // TCP header
        0xD4, 0x31, 0x01, 0xBB, // src port 54321, dst port 443
        0x00, 0x00, 0x00, 0x00, // seq
        0x00, 0x00, 0x00, 0x00, // ack
        0x50, 0x12, 0xFF, 0xFF, // data offset=5, flags=0x12 (SYN|ACK)
        0x00, 0x00, 0x00, 0x00, // urg ptr + options
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.version, 4);
      expect(p.protocol, Protocol.tcp);
      expect(p.protocolNumber, 6);
      expect(p.totalLength, 40);
      expect(p.srcIpMasked, '10.0.0.0');
      expect(p.dstIpMasked, '93.184.216.0');
      expect(p.srcPort, 54321);
      expect(p.dstPort, 443);
      expect(p.tcpFlags, 0x12);
    });

    test('IPv4 UDP — ports, no flags', () {
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x1C, // total length = 28
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x11, 0x00, 0x00, // proto=17 (UDP)
        0xC0, 0xA8, 0x01, 0x64, // src 192.168.1.100
        0x08, 0x08, 0x08, 0x08, // dst 8.8.8.8
        // UDP header
        0xE9, 0xC4, 0x00, 0x35, // src port 59844, dst port 53
        0x00, 0x08, 0x00, 0x00, // length=8, checksum
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.protocol, Protocol.udp);
      expect(p.srcPort, 59844);
      expect(p.dstPort, 53);
      expect(p.tcpFlags, isNull);
    });

    test('IPv4 ICMP — masked IPs, no ports', () {
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x14, // total length = 20 (header only)
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x01, 0x00, 0x00, // proto=1 (ICMP)
        0x01, 0x02, 0x03, 0x04, // src
        0x05, 0x06, 0x07, 0x08, // dst
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.protocol, Protocol.icmp);
      expect(p.srcPort, isNull);
      expect(p.dstPort, isNull);
      expect(p.srcIpMasked, '1.2.3.0');
      expect(p.dstIpMasked, '5.6.7.0');
    });

    test('IPv4 unknown protocol (47 = GRE) — masked IPs, no ports', () {
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x14,
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x2F, 0x00, 0x00, // proto=47 (GRE)
        0x0A, 0x00, 0x00, 0x01,
        0x0A, 0x00, 0x00, 0x02,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.protocol, Protocol.other);
      expect(p.protocolNumber, 47);
      expect(p.srcPort, isNull);
      expect(p.dstPort, isNull);
    });

    test('IPv6 TCP — masked /48 IPs + ports + flags', () {
      // 40-byte IPv6 header + 20-byte TCP header.
      // 2001:db8:85a3::8a2e:370:7334 → 2606:4700:4700::1111, port 80.
      final bytes = Uint8List.fromList([
        0x60, 0x00, 0x00, 0x00, // ver=6, tc=0, flow=0
        0x00, 0x14, // payload length = 20 (TCP header)
        0x06, 0x40, // next=TCP(6), hop=64
        // src 2001:db8:85a3:0:8a2e:370:7334 (rfc3849 doc prefix,
        // host part 8a2e:370:7334) — 8 hextets = 16 bytes.
        0x20, 0x01, 0x0d, 0xb8, 0x85, 0xa3,
        0x00, 0x00, 0x8a, 0x2e, 0x03, 0x70, 0x73, 0x34,
        0x00, 0x00, // trailing zeros for ::...0 hextet
        // dst 2606:4700:4700::1111 — 8 hextets = 16 bytes.
        0x26, 0x06, 0x47, 0x00, 0x47, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x11, 0x11,
        0x00, 0x00, // trailing zeros for ::...0 hextet
        // TCP header
        0xC0, 0x00, 0x00, 0x50, // src 49152, dst 80
        0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00,
        0x50, 0x18, 0xFF, 0xFF, // data offset=5, flags=0x18 (PSH|ACK)
        0x00, 0x00, 0x00, 0x00,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.version, 6);
      expect(p.protocol, Protocol.tcp);
      expect(p.srcPort, 49152);
      expect(p.dstPort, 80);
      expect(p.tcpFlags, 0x18);
      // /48 mask: keep first 3 hextets, zero the rest.
      expect(p.srcIpMasked, '2001:db8:85a3:0:0:0:0:0');
      expect(p.dstIpMasked, '2606:4700:4700:0:0:0:0:0');
    });

    test('IPv6 UDP — masked /48 IPs + ports, no flags', () {
      final bytes = Uint8List.fromList([
        0x60, 0x00, 0x00, 0x00,
        0x00, 0x08, // payload length = 8 (UDP header)
        0x11, 0x40, // next=UDP(17)
        0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // src ::1
        0xff, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, // dst ff02::1
        // UDP header
        0xE9, 0xC4, 0x14, 0xE9, // src 59844, dst 5353 (mDNS)
        0x00, 0x08, 0x00, 0x00,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.protocol, Protocol.udp);
      expect(p.srcPort, 59844);
      expect(p.dstPort, 5353);
      expect(p.tcpFlags, isNull);
      // /48 mask: fe80:0000:0000 → "fe80:0:0"
      expect(p.srcIpMasked, 'fe80:0:0:0:0:0:0:0');
      expect(p.dstIpMasked, 'ff02:0:0:0:0:0:0:0');
    });

    test('IPv6 + Hop-by-Hop extension header — walks past to TCP', () {
      // 40-byte IPv6 header (next=Hop-by-Hop=0) + 8-byte HBH option
      // header (extLen=0 → 8 bytes total) + 20-byte TCP header.
      final bytes = Uint8List.fromList([
        0x60, 0x00, 0x00, 0x00,
        0x00, 0x1C, // payload length = 28 (HBH 8 + TCP 20)
        0x00, 0x40, // next=Hop-by-Hop(0), hop=64
        0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
        0x20, 0x01, 0x0d, 0xb8, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
        // HBH header: next=TCP(6), extLen=0 (1*8=8 bytes total)
        0x06, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        // TCP header
        0xC0, 0x00, 0x00, 0x50,
        0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00,
        0x50, 0x02, 0xFF, 0xFF, // flags=0x02 (SYN)
        0x00, 0x00, 0x00, 0x00,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.version, 6);
      expect(p.protocol, Protocol.tcp);
      expect(p.srcPort, 49152);
      expect(p.dstPort, 80);
      expect(p.tcpFlags, 0x02);
    });

    test('Empty buffer returns null', () {
      expect(PacketParser.parse(Uint8List(0)), isNull);
    });

    test('Truncated IPv4 header (< 20 bytes) returns null', () {
      final bytes = Uint8List.fromList([0x45, 0x00, 0x00, 0x00]);
      expect(PacketParser.parse(bytes), isNull);
    });

    test('Truncated L4 — IP header ok, but no port bytes', () {
      // 20-byte IPv4 + 4 bytes (not enough for full TCP/UDP header).
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x18, // total length = 24
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x06, 0x00, 0x00,
        0x0A, 0x00, 0x00, 0x01,
        0x0A, 0x00, 0x00, 0x02,
        0xC0, 0x00, 0x00, 0x50, // only 4 L4 bytes
      ]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      // Ports are null because the parser only reads ports when it
      // sees ≥ 8 (UDP) or ≥ 20 (TCP) bytes of L4.
      expect(p!.srcPort, isNull);
      expect(p.dstPort, isNull);
    });

    test('Invalid IHL (0) returns null', () {
      final bytes = Uint8List.fromList([
        0x40, 0x00, 0x00, 0x14, // ver=4, ihl=0
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x06, 0x00, 0x00,
        0x0A, 0x00, 0x00, 0x01,
        0x0A, 0x00, 0x00, 0x02,
      ]);
      expect(PacketParser.parse(bytes), isNull);
    });

    test('Invalid IHL (4, below min 5) returns null', () {
      final bytes = Uint8List.fromList([
        0x44, 0x00, 0x00, 0x14, // ihl=4
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x06, 0x00, 0x00,
        0x0A, 0x00, 0x00, 0x01,
        0x0A, 0x00, 0x00, 0x02,
      ]);
      expect(PacketParser.parse(bytes), isNull);
    });

    test('IPv4 /24 mask — 192.168.1.42 → 192.168.1.0', () {
      // Construct a valid IPv4/TCP packet where the src IP is the
      // exact byte sequence 192.168.1.42 to verify the mask.
      final bytes = Uint8List.fromList([
        0x45, 0x00, 0x00, 0x28,
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x06, 0x00, 0x00,
        0xC0, 0xA8, 0x01, 0x2A, // 192.168.1.42
        0x0A, 0x00, 0x00, 0x01,
        0xC0, 0x00, 0x00, 0x50,
        0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00,
        0x50, 0x10, 0xFF, 0xFF,
        0x00, 0x00, 0x00, 0x00,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p!.srcIpMasked, '192.168.1.0');
    });

    test('IPv6 /48 mask — 2001:db8:1234:5678::1 → 2001:db8:1234:0:0:0:0:0', () {
      final bytes = Uint8List.fromList([
        0x60, 0x00, 0x00, 0x00,
        0x00, 0x14,
        0x06, 0x40,
        // src: 2001:db8:1234:5678:0:0:0:1
        0x20, 0x01, 0x0d, 0xb8, 0x12, 0x34, 0x56, 0x78,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
        // dst: any
        0xfe, 0x80, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02,
        // TCP
        0xC0, 0x00, 0x00, 0x50,
        0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00,
        0x50, 0x10, 0xFF, 0xFF,
        0x00, 0x00, 0x00, 0x00,
      ]);
      final p = PacketParser.parse(bytes);
      expect(p!.srcIpMasked, '2001:db8:1234:0:0:0:0:0');
    });

    test('Buffer larger than header — extra bytes are ignored', () {
      // 20-byte IPv4 + 20-byte TCP + 50 bytes of arbitrary "payload".
      // The parser MUST NOT look past the L4 header.
      final header = [
        0x45, 0x00, 0x00, 0x4A, // total length = 74 (20 + 20 + 34)
        0x00, 0x00, 0x00, 0x00,
        0x40, 0x06, 0x00, 0x00,
        0x0A, 0x00, 0x00, 0x01,
        0x0A, 0x00, 0x00, 0x02,
        0xC0, 0x00, 0x00, 0x50,
        0x00, 0x00, 0x00, 0x00,
        0x00, 0x00, 0x00, 0x00,
        0x50, 0x18, 0xFF, 0xFF,
        0x00, 0x00, 0x00, 0x00,
      ];
      final payload = List<int>.filled(50, 0xAB);
      final bytes = Uint8List.fromList([...header, ...payload]);
      final p = PacketParser.parse(bytes);
      expect(p, isNotNull);
      expect(p!.srcPort, 49152);
      expect(p.dstPort, 80);
      // Privacy: we do NOT expose any payload field. The toJson()
      // map has no payload key, by design.
      expect(p.toJson().keys, isNot(contains('payload')));
      expect(p.toJson().keys, isNot(contains('tlsClientHelloFingerprint')));
    });
  });
}
