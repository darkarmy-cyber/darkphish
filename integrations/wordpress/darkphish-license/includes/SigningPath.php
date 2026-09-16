<?php
declare(strict_types=1);
namespace Darkphish\Licensing;

final class SigningPath {
    public static function requireDirectAbsolute(string $path): void {
        $normalized = str_replace('\\', '/', $path);
        if (str_contains($path, "\0") ||
            (!str_starts_with($normalized, '/') && !preg_match('~^[A-Za-z]:/~', $normalized)) ||
            str_starts_with($normalized, '//') || preg_match('~(?:^|/)\.{1,2}(?:/|$)~', $normalized)) {
            throw new \RuntimeException('A direct absolute signing path is required');
        }
        // Do not discard a public alias by resolving it before inspection.
        // Reject file and directory symlinks, including private aliases, so the
        // configured pathname and every parent name the actual filesystem path.
        for ($part = $path; ; $part = $parent) {
            clearstatcache(true, $part);
            if (is_link($part)) { throw new \RuntimeException('Signing path must not contain symlinks'); }
            $parent = dirname($part);
            if ($parent === $part) { break; }
        }
    }
}
