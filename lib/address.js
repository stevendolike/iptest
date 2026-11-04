import { isIP } from 'node:net';

export function parseIPPort(ip, port) {
  if (typeof ip !== 'string') return null;
  const host = ip.trim();
  // Scope IDs are not accepted by the Go input parser or public-IP workflow.
  if (!isIP(host) || host.includes('%')) return null;
  const text = String(port).trim();
  if (!/^\d+$/.test(text)) return null;
  const value = Number(text);
  if (!Number.isSafeInteger(value) || value < 1 || value > 65535) return null;
  return { ip: host, port: value };
}

// Call after parseIPPort: IPv6 needs brackets to keep the port unambiguous.
export function formatIPPort(ip, port) {
  return `${isIP(ip) === 6 ? `[${ip}]` : ip}:${port}`;
}
