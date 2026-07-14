/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  // The automate UI is served under /automate via the app-router folder
  // structure (src/app/automate/*), so no basePath rewrite is needed.
};

export default nextConfig;
