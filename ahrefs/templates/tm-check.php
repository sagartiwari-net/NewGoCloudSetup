<?php
/**
 * ToolsMandi Ahrefs Bar — aMember membership check (upload to aMember host)
 *
 * Place NEXT TO library/:
 *   tm-check.php
 *   library/Am/Lite.php
 *
 * Panel (upays) and aMember can be on different servers — only this file
 * must live on the aMember site. Set its public URL in panel → Portals.
 *
 * Optional: TM_CHECK_SECRET + panel portal shared secret → header X-TM-Check-Key
 */

declare(strict_types=1);

header('Content-Type: application/json; charset=utf-8');
header('Cache-Control: no-store');

const TM_CHECK_SECRET = '';
const TM_AMEMBER_LITE_PATH = '';
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

    $dir = str_replace('\\', '/', __DIR__);
    $parent = dirname($dir);
    $grand = dirname($parent);
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
        'Expected library/Am/Lite.php next to this tm-check.php. Or set TM_AMEMBER_LITE_PATH.'
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

/**
 * aMember Pro: isLoggedIn / getUser / haveSubscriptions are INSTANCE methods.
 * Official usage: Am_Lite::getInstance()->isLoggedIn()
 * Never call Am_Lite::isLoggedIn() statically (PHP 8 fatals).
 *
 * @return Am_Lite
 */
function tm_am_lite_instance()
{
    if (!class_exists('Am_Lite')) {
        tm_fail('amember_lite_missing', 500, 'Am_Lite class missing');
    }

    $errs = [];

    if (method_exists('Am_Lite', 'getInstance')) {
        try {
            $inst = Am_Lite::getInstance();
            if (is_object($inst)) {
                return $inst;
            }
            $errs[] = 'getInstance returned non-object';
        } catch (Throwable $e) {
            $errs[] = 'getInstance: ' . $e->getMessage();
        }
    }

    if (method_exists('Am_Lite', 'i')) {
        try {
            $inst = Am_Lite::i();
            if (is_object($inst)) {
                return $inst;
            }
            $errs[] = 'i() returned non-object';
        } catch (Throwable $e) {
            $errs[] = 'i(): ' . $e->getMessage();
        }
    }

    try {
        $inst = new Am_Lite();
        if (is_object($inst)) {
            return $inst;
        }
        $errs[] = 'new Am_Lite returned non-object';
    } catch (Throwable $e) {
        $errs[] = 'new Am_Lite: ' . $e->getMessage();
    }

    tm_fail('amember_lite_missing', 500, 'No Am_Lite instance: ' . implode(' | ', $errs));
}

/** @param object $am */
function tm_am_call($am, string $method, array $args = [])
{
    if (!is_object($am)) {
        throw new RuntimeException('Am_Lite instance required for ' . $method);
    }
    if (!method_exists($am, $method)) {
        throw new RuntimeException('Am_Lite method missing: ' . $method);
    }
    return call_user_func_array([$am, $method], $args);
}

try {
    // Adopt session cookie from bar-api (different server) before Am_Lite reads auth.
    if ($token !== '') {
        $names = [];
        if ($cookieName !== '') {
            $names[] = $cookieName;
        }
        $names = array_merge($names, [
            'PHPSESSID', 'amember_nr', 'amember_ru', 'amember_r', 'amember_sess', 'amember',
        ]);
        foreach (array_values(array_unique($names)) as $n) {
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

    $am = tm_am_lite_instance();

    if (!tm_am_call($am, 'isLoggedIn')) {
        tm_fail('not_logged_in');
    }

    $user = tm_am_call($am, 'getUser');
    if (!$user) {
        tm_fail('not_logged_in');
    }

    if ($products) {
        $okProduct = false;
        $productIds = array_values(array_filter(array_map('intval', $products)));
        try {
            if ($productIds) {
                $search = count($productIds) === 1 ? $productIds[0] : $productIds;
                if (tm_am_call($am, 'haveSubscriptions', [$search])) {
                    $okProduct = true;
                }
            }
        } catch (Throwable $ignored) {
        }
        if (!$okProduct) {
            foreach ($products as $pid) {
                foreach (['hasActiveProduct', 'hasProduct', 'hasSubscription'] as $m) {
                    try {
                        $arg = ($m === 'hasSubscription') ? (int) $pid : $pid;
                        if (tm_am_call($am, $m, [$arg])) {
                            $okProduct = true;
                            break 2;
                        }
                    } catch (Throwable $ignored) {
                    }
                }
            }
        }
        if (!$okProduct) {
            tm_fail('no_product', 200, 'Active subscription does not include product_ids=' . implode(',', $products));
        }
    }

    if (is_object($user)) {
        $user = (array) $user;
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
