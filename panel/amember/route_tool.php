<?php
/**
 * aMember button → panel handoff.
 *
 * 1. In the panel, Allowed sites → add this aMember domain and copy the key once.
 * 2. Paste that domain and key below.
 * 3. On a website, set Button name (for example refs).
 * 4. Map the aMember product ids on that website.
 * 5. Button: <a href="/route_tool.php?tool=refs" target="_blank">Open</a>
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
        'signature' => hash_hmac('sha256', $canonical, PANEL_SITE_KEY),
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
    $safe = htmlspecialchars($message, ENT_QUOTES, 'UTF-8');
    echo '<!DOCTYPE html><html lang="en"><head><meta charset="utf-8"><title>Access</title></head><body style="font-family:sans-serif;padding:2rem">';
    echo '<p>' . $safe . '</p>';
    echo '<p><a href="/member">Back</a></p></body></html>';
    exit;
}
