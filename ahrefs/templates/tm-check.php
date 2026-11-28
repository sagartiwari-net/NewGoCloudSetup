<?php
/**
 * ToolsMandi Ahrefs Bar — aMember membership check endpoint
 *
 * Install next to aMember's library/ folder (same directory level):
 *   …/tm-check.php
 *   …/library/Am/Lite.php
 *
 * Member URL path differs per site (/member, /amember, web root, …).
 * Put this file wherever Lite.php's parent tree lives, then set that
 * full URL as Portal "tm-check URL" in the panel.
 *
 * Optional shared secret: set TM_CHECK_SECRET below AND the same value
 * on the portal row (panel). Caller must send header X-TM-Check-Key.
 *
 * Request: Cookie header and/or JSON/query token + products
 * Response:
 *   { "ok": true, "user": { "login", "email", "name", "uid" } }
 *   { "ok": false, "error": "not_logged_in"|"no_product"|... }
 */

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');

const TM_CHECK_SECRET = ''; // e.g. 'change-me-long-random'
/**
 * Optional absolute path to Lite.php OR aMember root (folder containing library/).
 * Leave empty — auto-detect from this file's directory first.
 */
const TM_AMEMBER_LITE_PATH = '';
/** Comma-separated aMember product IDs that grant Ahrefs Bar access (fallback if request omits them). */
const TM_DEFAULT_PRODUCT_IDS = '';

function tm_fail(string $error, int $http = 200, string $detail = ''): void
{
    if ($http !== 200) {
        http_response_code($http);
    }
    $out = ['ok' => false, 'error' => $error];
    if ($detail !== '') {
        $out['detail'] = $detail;
    }
    echo json_encode($out);
    exit;
}

if (TM_CHECK_SECRET !== '') {
    $key = $_SERVER['HTTP_X_TM_CHECK_KEY'] ?? '';
    if (!hash_equals(TM_CHECK_SECRET, (string) $key)) {
        tm_fail('forbidden', 403);
    }
}

/**
 * Locate library/Am/Lite.php.
 * Primary rule: tm-check.php and library/ are siblings (same folder).
 * Extra candidates cover odd layouts without hardcoding one reseller domain.
 */
function tm_find_lite_path(): string
{
    if (TM_AMEMBER_LITE_PATH !== '') {
        $p = rtrim(str_replace('\\', '/', TM_AMEMBER_LITE_PATH), '/');
        if (is_file($p) && strtolower(substr($p, -8)) === 'lite.php') {
            return $p;
        }
        if (is_file($p . '/library/Am/Lite.php')) {
            return $p . '/library/Am/Lite.php';
        }
    }

    if (defined('AMEMBER_LITE_PATH') && AMEMBER_LITE_PATH !== '') {
        $p = rtrim(str_replace('\\', '/', (string) AMEMBER_LITE_PATH), '/');
        if (is_file($p) && strtolower(substr($p, -8)) === 'lite.php') {
            return $p;
        }
        if (is_file($p . '/library/Am/Lite.php')) {
            return $p . '/library/Am/Lite.php';
        }
    }

    $dir = str_replace('\\', '/', __DIR__);
    $parent = dirname($dir);
    $grand = dirname($parent);

    // Order matters: same-level library/ wins (normal install).
    $candidates = [
        $dir . '/library/Am/Lite.php',
        $dir . '/amember/library/Am/Lite.php',
        $dir . '/member/library/Am/Lite.php',
        $dir . '/members/library/Am/Lite.php',
        $parent . '/library/Am/Lite.php',
        $parent . '/amember/library/Am/Lite.php',
        $parent . '/member/library/Am/Lite.php',
        $grand . '/library/Am/Lite.php',
        $grand . '/amember/library/Am/Lite.php',
        $grand . '/member/library/Am/Lite.php',
    ];

    // dirname(__DIR__, n) style extras when PHP supports it (already covered above).
    foreach ([1, 2, 3] as $n) {
        $base = dirname(__DIR__, $n);
        $candidates[] = $base . '/library/Am/Lite.php';
        $candidates[] = $base . '/amember/library/Am/Lite.php';
        $candidates[] = $base . '/member/library/Am/Lite.php';
    }

    $seen = [];
    foreach ($candidates as $path) {
        $path = str_replace('\\', '/', $path);
        if (isset($seen[$path])) {
            continue;
        }
        $seen[$path] = true;
        if (is_file($path)) {
            return $path;
        }
    }
    return '';
}

$lite = tm_find_lite_path();
if ($lite === '') {
    tm_fail(
        'amember_lite_missing',
        500,
        'Expected library/Am/Lite.php next to tm-check.php (same folder). Or set TM_AMEMBER_LITE_PATH.'
    );
}
require_once $lite;

$raw = file_get_contents('php://input') ?: '';
$in = [];
if ($raw !== '') {
    $decoded = json_decode($raw, true);
    if (is_array($decoded)) {
        $in = $decoded;
    }
}
$token = (string) ($in['token'] ?? $_REQUEST['token'] ?? '');
$cookieName = (string) ($in['cookie_name'] ?? $_REQUEST['cookie_name'] ?? '');
$productCsv = (string) (
    $in['product_ids']
    ?? $in['products']
    ?? $_REQUEST['product_ids']
    ?? $_REQUEST['products']
    ?? TM_DEFAULT_PRODUCT_IDS
);
$products = array_values(array_filter(array_map('trim', explode(',', $productCsv)), 'strlen'));

try {
    if (!class_exists('Am_Lite')) {
        tm_fail('amember_lite_missing', 500, 'Am_Lite class missing after require: ' . $lite);
    }

    // Adopt session cookie from bar-api before Am_Lite reads login state.
    if ($token !== '') {
        $names = [];
        if ($cookieName !== '') {
            $names[] = $cookieName;
        }
        $names = array_merge($names, [
            'PHPSESSID', 'amember_nr', 'amember_ru', 'amember_r', 'amember_sess', 'amember',
        ]);
        $names = array_values(array_unique($names));
        foreach ($names as $n) {
            if (empty($_COOKIE[$n])) {
                $_COOKIE[$n] = $token;
            }
        }
        if (session_status() === PHP_SESSION_NONE) {
            $sidName = $cookieName !== '' ? $cookieName : 'PHPSESSID';
            if ($sidName === 'PHPSESSID' || $sidName === '') {
                @session_id($token);
            }
        }
    }

    if (!Am_Lite::isLoggedIn()) {
        tm_fail('not_logged_in');
    }

    $user = Am_Lite::getUser();
    if (!$user) {
        tm_fail('not_logged_in');
    }

    if ($products) {
        $okProduct = false;
        foreach ($products as $pid) {
            if (
                (method_exists('Am_Lite', 'hasActiveProduct') && Am_Lite::hasActiveProduct($pid))
                || (method_exists('Am_Lite', 'hasProduct') && Am_Lite::hasProduct($pid))
            ) {
                $okProduct = true;
                break;
            }
        }
        if (!$okProduct && method_exists('Am_Lite', 'hasSubscription')) {
            foreach ($products as $pid) {
                if (Am_Lite::hasSubscription((int) $pid)) {
                    $okProduct = true;
                    break;
                }
            }
        }
        if (!$okProduct && method_exists('Am_Lite', 'haveSubscriptions')) {
            try {
                if (Am_Lite::haveSubscriptions($products)) {
                    $okProduct = true;
                }
            } catch (Throwable $ignored) {
            }
        }
        if (!$okProduct) {
            tm_fail('no_product', 200, 'Active subscription does not include product_ids=' . implode(',', $products));
        }
    }

    $login = (string) ($user['login'] ?? $user['username'] ?? '');
    $email = (string) ($user['email'] ?? '');
    $name = trim((string) ($user['name_f'] ?? '') . ' ' . (string) ($user['name_l'] ?? ''));
    $uid = (int) ($user['user_id'] ?? $user['id'] ?? 0);

    echo json_encode([
        'ok' => true,
        'user' => [
            'login' => $login,
            'email' => $email,
            'name' => $name !== '' ? $name : $login,
            'uid' => $uid,
        ],
    ]);
} catch (Throwable $e) {
    tm_fail('check_failed', 500, $e->getMessage());
}
