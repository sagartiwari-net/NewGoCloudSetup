<?php
/**
 * ToolsMandi Ahrefs Bar — aMember membership check endpoint
 *
 * Prefer installing THIS file inside the aMember root, e.g.:
 *   https://yoursite.com/member/tm-check.php
 * then set that URL as Portal "tm-check URL".
 *
 * If you keep it at the site web root, aMember must live in ./member or
 * set TM_AMEMBER_ROOT below to the folder that contains library/Am/Lite.php.
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
/** Absolute path to aMember root (folder that contains library/Am/Lite.php). Leave empty to auto-detect. */
const TM_AMEMBER_ROOT = '';
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
 * Resolve aMember root. toolsmandi-style installs often live in /member
 * while operators drop tm-check.php at the domain web root.
 */
function tm_find_amember_root(): string
{
    if (TM_AMEMBER_ROOT !== '' && is_file(rtrim(TM_AMEMBER_ROOT, '/\\') . '/library/Am/Lite.php')) {
        return rtrim(TM_AMEMBER_ROOT, '/\\');
    }
    $candidates = [
        __DIR__,
        __DIR__ . '/member',
        dirname(__DIR__),
        dirname(__DIR__) . '/member',
        dirname(__DIR__) . '/public_html/member',
        dirname(__DIR__) . '/httpdocs/member',
    ];
    foreach ($candidates as $dir) {
        if (is_file($dir . '/library/Am/Lite.php')) {
            return $dir;
        }
    }
    return '';
}

$amRoot = tm_find_amember_root();
if ($amRoot === '') {
    tm_fail('amember_lite_missing', 500, 'Put tm-check.php inside aMember root (e.g. /member/) or set TM_AMEMBER_ROOT');
}
$lite = $amRoot . '/library/Am/Lite.php';
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
        tm_fail('amember_lite_missing', 500, 'Am_Lite class missing after require');
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
            // Prefer real PHP session id when the portal cookie is PHPSESSID.
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
            // Some builds expose haveSubscriptions(array $ids): bool
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
