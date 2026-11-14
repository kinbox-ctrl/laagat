/** @type {import('next').NextConfig} */
const api = process.env.API_URL || "http://127.0.0.1:8080";

export default {
  // The Go API is the only backend; the browser talks to it through this
  // same-origin proxy so the session cookie stays first-party.
  async rewrites() {
    return [{ source: "/api/:path*", destination: `${api}/api/:path*` }];
  },
};
