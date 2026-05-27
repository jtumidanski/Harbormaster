export function readCsrfCookie(): string {
  const m = document.cookie.match(/(?:^|;\s*)harbormaster_csrf=([^;]+)/);
  return m ? decodeURIComponent(m[1]) : "";
}
