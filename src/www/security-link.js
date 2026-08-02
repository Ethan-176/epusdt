"use strict";
(() => {
  if (location.pathname === "/account/password") {
    location.replace("/admin-security.html");
    return;
  }
  if (location.pathname.startsWith("/cashier") || location.pathname === "/sign-in" || location.pathname === "/admin-security.html") return;
  if (!document.cookie.split("; ").some((row) => row.startsWith("access_token="))) return;
  const removeLegacyPasswordLinks = () => {
    document.querySelectorAll('a[href="/account/password"]').forEach((node) => node.remove());
  };
  removeLegacyPasswordLinks();
  new MutationObserver(removeLegacyPasswordLinks).observe(document.documentElement, { childList: true, subtree: true });
  const link = document.createElement("a");
  link.href = "/admin-security.html"; link.textContent = "安全设置";
  Object.assign(link.style, { position: "fixed", right: "20px", bottom: "20px", zIndex: "9999", padding: "9px 13px", borderRadius: "999px", background: "#18181b", color: "#fafafa", border: "1px solid #3f3f46", font: "600 13px system-ui", textDecoration: "none", boxShadow: "0 8px 24px #0005" });
  document.body.appendChild(link);
})();
