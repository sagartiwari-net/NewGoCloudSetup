<?php
/**
 * ToolsMandi Ahrefs Bar — aMember membership check endpoint
 *
 * Install on the member site (same host as aMember), then set this URL
 * as Portal "tm-check URL" in the panel Ahrefs Bar → Portals screen.
 *
 * Optional shared secret: set TM_CHECK_SECRET below AND the same value
 * on the portal row (panel). Caller must send header X-TM-Check-Key.
 *
 * Expected query/body from bar-api:
 *   cookie / token, product_ids
 * Response JSON:
 *   { "ok": true, "user": { "login": "...", "email": "...", "name": "...", "uid": 123 } }
 *   or { "ok": false, "error": "not_logged_in"|"no_product"|... }
 */

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');

const TM_CHECK_SECRET = ''; // e.g. 'change-me-long-random'
/** Comma-separated aMember product IDs that grant Ahrefs Bar access (fallback if request omits them). */
const TM_DEFAULT_PRODUCT_IDS = '';

if (TM_CHECK_SECRET !== '') {
    $key = $_SERVER['HTTP_X_TM_CHECK_KEY'] ?? '';
    if (!hash_equals(TM_CHECK_SECRET, (string) $key)) {
        http_response_code(403);
        echo json_encode(['ok' => false, 'error' => 'forbidden']);
        exit;
    }
}

// Bootstrap aMember — adjust path for your install.
$amRoot = dirname(__DIR__); // often /path/to/amember
if (!is_file($amRoot . '/library/Am/Lite.php') && is_file(__DIR__ . '/library/Am/Lite.php')) {
    $amRoot = __DIR__;
}
$lite = $amRoot . '/library/Am/Lite.php';
if (!is_file($lite)) {
    http_response_code(500);
    echo json_encode(['ok' => false, 'error' => 'amember_lite_missing']);
    exit;
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
// bar-api sends ?products=…; accept product_ids too (docs / older copies).
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
        throw new RuntimeException('Am_Lite missing');
    }
    // If bar-api forwarded a session cookie value, try to adopt it.
    if ($token !== '' && empty($_COOKIE['PHPSESSID'])) {
        $_COOKIE['PHPSESSID'] = $token;
    }

    if (!Am_Lite::isLoggedIn()) {
        echo json_encode(['ok' => false, 'error' => 'not_logged_in']);
        exit;
    }

    $user = Am_Lite::getUser();
    if (!$user) {
        echo json_encode(['ok' => false, 'error' => 'not_logged_in']);
        exit;
    }

    if ($products) {
        $okProduct = false;
        foreach ($products as $pid) {
            if (Am_Lite::hasActiveProduct($pid) || Am_Lite::hasProduct($pid)) {
                $okProduct = true;
                break;
            }
        }
        // Some aMember versions only expose hasSubscription / getAccess
        if (!$okProduct && method_exists('Am_Lite', 'hasSubscription')) {
            foreach ($products as $pid) {
                if (Am_Lite::hasSubscription((int) $pid)) {
                    $okProduct = true;
                    break;
                }
            }
        }
        if (!$okProduct) {
            echo json_encode(['ok' => false, 'error' => 'no_product']);
            exit;
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
    http_response_code(500);
    echo json_encode(['ok' => false, 'error' => 'check_failed']);
}
