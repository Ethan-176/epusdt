"use strict";
(() => {
  const $ = (id) => document.getElementById(id);

  function accessToken() {
    const row = document.cookie.split("; ").find((item) => item.startsWith("access_token="));
    if (!row) return "";
    try { return JSON.parse(decodeURIComponent(row.slice(row.indexOf("=") + 1))); } catch { return ""; }
  }

  async function api(path, options = {}, authenticated = false) {
    const headers = { "Content-Type": "application/json", ...(options.headers || {}) };
    if (authenticated) {
      const token = accessToken();
      if (!token) throw new Error("请先使用动态口令或通行密钥登录");
      headers.Authorization = `Bearer ${token}`;
    }
    const response = await fetch(`/admin/api/v1${path}`, { ...options, headers });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok || payload.status_code !== 200) throw new Error(payload.message || `请求失败 (${response.status})`);
    return payload.data;
  }

  function setMessage(message, error = false) {
    const element = $("passkey-message");
    element.textContent = message || "";
    element.classList.toggle("error", error);
  }

  function decode(value) {
    const base64 = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
    return Uint8Array.from(atob(base64), (char) => char.charCodeAt(0));
  }
  function encode(value) {
    const bytes = new Uint8Array(value); let binary = "";
    bytes.forEach((byte) => { binary += String.fromCharCode(byte); });
    return btoa(binary).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  }
  function creationOptions(value) {
    value.challenge = decode(value.challenge); value.user.id = decode(value.user.id);
    value.excludeCredentials = (value.excludeCredentials || []).map((item) => ({ ...item, id: decode(item.id) }));
    return value;
  }
  function serializeCredential(credential) {
    const response = credential.response;
    const result = { id: credential.id, rawId: encode(credential.rawId), type: credential.type, response: {} };
    for (const key of ["clientDataJSON", "attestationObject", "authenticatorData", "signature", "userHandle"]) {
      if (response[key]) result.response[key] = encode(response[key]);
    }
    if (response.getTransports) result.response.transports = response.getTransports();
    if (response.getAuthenticatorData) result.response.authenticatorData = encode(response.getAuthenticatorData());
    if (response.getPublicKey) result.response.publicKey = encode(response.getPublicKey());
    if (response.getPublicKeyAlgorithm) result.response.publicKeyAlgorithm = response.getPublicKeyAlgorithm();
    result.authenticatorAttachment = credential.authenticatorAttachment;
    result.clientExtensionResults = credential.getClientExtensionResults();
    return result;
  }

  async function refreshPasskeys() {
    const list = $("passkey-list");
    if (!accessToken()) {
      list.innerHTML = "<p>登录后可查看和删除已注册的通行密钥。</p>";
      return;
    }
    const rows = await api("/auth/passkeys", {}, true);
    list.textContent = "";
    if (!rows.length) { list.innerHTML = "<p>尚未注册通行密钥。</p>"; return; }
    rows.forEach((row) => {
      const item = document.createElement("div"); item.className = "passkey";
      const text = document.createElement("div");
      text.innerHTML = "<strong></strong><p></p>"; text.querySelector("strong").textContent = row.name;
      text.querySelector("p").textContent = `注册：${row.created_at || "-"} · 最近使用：${row.last_used_at || "未使用"}`;
      const button = document.createElement("button"); button.className = "danger"; button.textContent = "删除";
      button.onclick = async () => {
        const totpCode = prompt("请输入 6 位动态口令"); if (!totpCode) return;
        try {
          await api(`/auth/passkeys/${row.id}`, { method: "DELETE", body: JSON.stringify({ totp_code: totpCode }) }, true);
          location.href = "/sign-in";
        } catch (error) { alert(error.message); }
      };
      item.append(text, button); list.append(item);
    });
  }

  $("passkey-register").onclick = async () => {
    setMessage("");
    if (!window.PublicKeyCredential) return setMessage("当前浏览器不支持通行密钥。", true);
    const username = $("passkey-username").value.trim();
    const totpCode = $("passkey-totp").value.trim();
    const name = $("passkey-name").value.trim();
    if (!username || totpCode.length !== 6 || !name) return setMessage("请填写登录名、6 位动态口令和密钥名称。", true);
    try {
      const start = await api("/auth/passkeys/register/start", {
        method: "POST", body: JSON.stringify({ username, totp_code: totpCode, name }),
      });
      const credential = await navigator.credentials.create({ publicKey: creationOptions(start.publicKey) });
      await api("/auth/passkeys/register/finish", {
        method: "POST", body: JSON.stringify({ username, challenge_id: start.challenge_id, credential: serializeCredential(credential) }),
      });
      $("passkey-totp").value = "";
      setMessage("通行密钥注册成功，现在可以返回登录页使用。", false);
      await refreshPasskeys();
    } catch (error) { setMessage(error.message || "通行密钥注册失败", true); }
  };

  refreshPasskeys().catch((error) => setMessage(error.message, true));
})();
