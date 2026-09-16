<?php
declare(strict_types=1);
// Test-process-only namespace shims; production calls native PHP functions.
namespace Darkphish\Licensing;
function fwrite($handle, string $data): int|false {
    return ($GLOBALS['keySetupFault'] ?? '') === 'write' ? \fwrite($handle, substr($data, 0, 4)) : \fwrite($handle, $data);
}
function fflush($handle): bool { return ($GLOBALS['keySetupFault'] ?? '') === 'flush' ? false : \fflush($handle); }
function fsync($handle): bool { return ($GLOBALS['keySetupFault'] ?? '') === 'sync' ? false : \fsync($handle); }
