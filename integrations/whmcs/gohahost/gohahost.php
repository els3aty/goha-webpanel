<?php
/**
 * GohaHost WHMCS Provisioning Module
 * 
 * Provides a secure API integration with the GohaHost Control Plane.
 */

if (!defined("WHMCS")) {
    die("This file cannot be accessed directly");
}

function gohahost_MetaData() {
    return array(
        'DisplayName' => 'GohaHost Control Panel',
        'APIVersion' => '1.1',
        'RequiresServer' => true,
    );
}

function gohahost_ConfigOptions() {
    return array(
        'Package Name' => array(
            'Type' => 'text',
            'Size' => '25',
            'Default' => 'default',
            'Description' => 'Enter the GohaHost package name',
        ),
    );
}

function gohahost_api_call($params, $endpoint, $postData) {
    $serverHost = $params['serverhostname'] ? $params['serverhostname'] : $params['serverip'];
    $apiToken = $params['serveraccesshash']; // We store the WHMCS API Token here
    
    $url = "https://" . $serverHost . "/api/whmcs/accounts/" . $endpoint;
    
    $ch = curl_init();
    curl_setopt($ch, CURLOPT_URL, $url);
    curl_setopt($ch, CURLOPT_POST, 1);
    curl_setopt($ch, CURLOPT_POSTFIELDS, json_encode($postData));
    curl_setopt($ch, CURLOPT_RETURNTRANSFER, 1);
    
    // GohaHost strictly uses Bearer tokens scoped to WHMCS
    $headers = array(
        'Authorization: Bearer ' . trim($apiToken),
        'Content-Type: application/json',
        'Accept: application/json'
    );
    curl_setopt($ch, CURLOPT_HTTPHEADER, $headers);
    curl_setopt($ch, CURLOPT_TIMEOUT, 30);
    
    // Enforce SSL verification for security
    curl_setopt($ch, CURLOPT_SSL_VERIFYHOST, 2);
    curl_setopt($ch, CURLOPT_SSL_VERIFYPEER, 1);
    
    $response = curl_exec($ch);
    $error = curl_error($ch);
    $httpCode = curl_getinfo($ch, CURLINFO_HTTP_CODE);
    curl_close($ch);
    
    if ($error) {
        return array('success' => false, 'error' => "cURL Error: " . $error);
    }
    
    $data = json_decode($response, true);
    if ($httpCode >= 400) {
        $msg = isset($data['error']) ? $data['error'] : "HTTP $httpCode";
        return array('success' => false, 'error' => "API Error: " . $msg);
    }
    
    return array('success' => true, 'data' => $data);
}

function gohahost_CreateAccount(array $params) {
    try {
        $postData = array(
            'username' => $params['username'],
            'password' => $params['password'],
            'email'    => $params['clientsdetails']['email'],
            'domain'   => $params['domain'],
            'package'  => $params['configoption1']
        );
        
        $res = gohahost_api_call($params, "create", $postData);
        if (!$res['success']) {
            return $res['error'];
        }
        
        return 'success';
    } catch (Exception $e) {
        return "Exception: " . $e->getMessage();
    }
}

function gohahost_SuspendAccount(array $params) {
    try {
        $postData = array('username' => $params['username']);
        $res = gohahost_api_call($params, "suspend", $postData);
        if (!$res['success']) {
            return $res['error'];
        }
        return 'success';
    } catch (Exception $e) {
        return "Exception: " . $e->getMessage();
    }
}

function gohahost_UnsuspendAccount(array $params) {
    try {
        $postData = array('username' => $params['username']);
        $res = gohahost_api_call($params, "unsuspend", $postData);
        if (!$res['success']) {
            return $res['error'];
        }
        return 'success';
    } catch (Exception $e) {
        return "Exception: " . $e->getMessage();
    }
}

function gohahost_TerminateAccount(array $params) {
    try {
        $postData = array('username' => $params['username']);
        $res = gohahost_api_call($params, "terminate", $postData);
        if (!$res['success']) {
            return $res['error'];
        }
        return 'success';
    } catch (Exception $e) {
        return "Exception: " . $e->getMessage();
    }
}
