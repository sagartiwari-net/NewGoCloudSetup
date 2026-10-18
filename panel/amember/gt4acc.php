<?php
/**
 * aMember button → panel handoff.
 *
 * 1. In the panel, Allowed sites → add this aMember domain and copy the key once.
 * 2. Paste that domain and key below.
 * 3. On a website, set Button name (for example refs).
 * 4. Map the aMember product ids on that website.
 * 5. On Allowed sites, check the tools this domain may open.
 * 6. Button: <a href="/gt4acc.php?tool=refs" target="_blank">Open</a>
 *
 * The signature is HMAC-SHA256 over these lines, joined with \n:
 * site, tool, username, sorted product ids, customer IP, unix time, nonce.
 */

const PANEL_HANDOFF_URL = 'https://panel.gt4rents.com/api/access/handoff';
const PANEL_SITE_DOMAIN = 'toolsmandi.com';
const PANEL_SITE_KEY = 'paste-the-key-from-allowed-sites';

error_reporting(E_ALL);
ini_set('display_errors', '0');

try {
    if (!defined('APPLICATION_PATH')) {
        $paths = [
            __DIR__ . '/bootstrap.php',
            __DIR__ . '/application/bootstrap.php',
            __DIR__ . '/member/bootstrap.php',
            __DIR__ . '/member/application/bootstrap.php',
            __DIR__ . '/amember/bootstrap.php',
            __DIR__ . '/amember/application/bootstrap.php',
            dirname(__DIR__) . '/bootstrap.php',
            dirname(__DIR__) . '/application/bootstrap.php',
        ];
        $loaded = false;
        foreach ($paths as $path) {
            if (file_exists($path)) {
                require_once $path;
                $loaded = true;
                break;
            }
        }
        if (!$loaded) {
            handoffFail('aMember could not be loaded.');
        }
    }

    $user = Am_Di::getInstance()->auth->getUser();
    if (!$user) {
        header('Location: /member/login');
        exit;
    }

    $tool = isset($_GET['tool']) ? strtolower(trim((string) $_GET['tool'])) : '';
    $tool = preg_replace('/[^a-z0-9-]/', '', $tool);
    if ($tool === '') {
        handoffFail('This tool is not available');
    }

    if (trim(PANEL_SITE_KEY) === '' || PANEL_SITE_KEY === 'paste-the-key-from-allowed-sites') {
        handoffFail('Paste the allowed-sites key into gt4acc.php.');
    }

    $username = trim((string) $user->login);
    $site = handoffSite(PANEL_SITE_DOMAIN);
    $productIds = array_values(array_unique(array_map('strval', (array) $user->getActiveProductIds())));
    sort($productIds, SORT_STRING);
    if (count($productIds) > 50) {
        $productIds = array_slice($productIds, 0, 50);
    }
    $clientIP = handoffClientIP();
    $timestamp = time();
    $nonce = bin2hex(random_bytes(16));
    $canonical = implode("\n", [
        $site,
        $tool,
        $username,
        implode(',', $productIds),
        $clientIP,
        (string) $timestamp,
        $nonce,
    ]);
    $payload = json_encode([
        'site' => $site,
        'tool' => $tool,
        'username' => $username,
        'product_ids' => $productIds,
        'client_ip' => $clientIP,
        'timestamp' => $timestamp,
        'nonce' => $nonce,
        'signature' => hash_hmac('sha256', $canonical, trim(PANEL_SITE_KEY)),
    ]);

    $context = stream_context_create([
        'http' => [
            'method' => 'POST',
            'header' => "Content-Type: application/json\r\n",
            'content' => $payload,
            'timeout' => 15,
            'ignore_errors' => true,
        ],
    ]);
    $response = @file_get_contents(PANEL_HANDOFF_URL, false, $context);
    $data = is_string($response) ? json_decode($response, true) : null;
    if (!is_array($data)) {
        handoffFail('The panel did not answer.');
    }
    if (!empty($data['open_url']) && is_string($data['open_url'])) {
        header('Location: ' . $data['open_url']);
        exit;
    }
    $message = !empty($data['message']) && is_string($data['message']) ? $data['message'] : 'Direct access not allowed';
    handoffFail($message);
} catch (Throwable $e) {
    handoffFail('The button could not be opened.');
}

function handoffSite(string $raw): string {
    $domain = strtolower(trim($raw));
    $domain = preg_replace('#^https?://#', '', $domain) ?? $domain;
    $domain = preg_replace('#^www\.#', '', $domain) ?? $domain;
    $cut = strcspn($domain, '/?#');
    $domain = substr($domain, 0, $cut);
    return rtrim($domain, '.');
}

function handoffClientIP(): string {
    $keys = ['HTTP_CF_CONNECTING_IP', 'HTTP_X_REAL_IP', 'HTTP_X_FORWARDED_FOR', 'REMOTE_ADDR'];
    foreach ($keys as $key) {
        if (empty($_SERVER[$key])) {
            continue;
        }
        $ip = trim(explode(',', (string) $_SERVER[$key])[0]);
        if (filter_var($ip, FILTER_VALIDATE_IP)) {
            return $ip;
        }
    }
    return '127.0.0.1';
}

function handoffFail(string $message): void {
    http_response_code(403);
    header('Content-Type: text/html; charset=utf-8');
    header('Cache-Control: no-store');
    $safe = htmlspecialchars($message, ENT_QUOTES, 'UTF-8');
    echo '<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Access Denied</title>';
    echo '<style>*{box-sizing:border-box;margin:0;padding:0}body{min-height:100vh;display:flex;align-items:center;justify-content:center;padding:24px;background:#eef3f8;color:#0f172a;font-family:system-ui,-apple-system,Segoe UI,sans-serif}.card{width:min(440px,100%);background:#fff;border-radius:28px;box-shadow:0 24px 60px rgba(15,23,42,.08);padding:48px 36px 36px;text-align:center}.ring{width:78px;height:78px;margin:0 auto 22px;border-radius:50%;background:conic-gradient(#3b82f6 0 70deg,#e7eef8 70deg 360deg);display:grid;place-items:center}.lock{width:64px;height:64px;border-radius:50%;background:#fff;display:grid;place-items:center;font-size:26px}h1{font-size:28px;line-height:1.2;font-weight:800;letter-spacing:-.03em;margin-bottom:12px}.msg{color:#64748b;font-size:15px;line-height:1.55}.back{margin-top:22px;display:inline-flex;align-items:center;padding:8px 16px;border:1px solid #e6ebf2;border-radius:999px;color:#334155;font-size:14px;text-decoration:none;background:#fff}</style>';
    echo '</head><body><div class="card"><div class="ring"><div class="lock">&#128274;</div></div>';
    echo '<h1>Access Denied</h1><p class="msg">' . $safe . '</p>';
    echo '<a class="back" href="/member">Back</a></div></body></html>';
    exit;
}
