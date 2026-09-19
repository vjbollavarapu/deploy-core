/** @type {import('next').NextConfig} */
const nextConfig = {
  images: {
    unoptimized: true,
  },
  async redirects() {
    return [
      { source: '/', destination: '/dashboard', permanent: false },
      { source: '/git-providers', destination: '/integrations/git', permanent: false },
      { source: '/registries', destination: '/integrations/registries', permanent: false },
      { source: '/notifications', destination: '/integrations/notifications', permanent: false },
      { source: '/webhooks', destination: '/integrations/webhooks', permanent: false },
      { source: '/secrets', destination: '/security/secrets', permanent: false },
      { source: '/teams', destination: '/security/access', permanent: false },
      { source: '/audit-log', destination: '/security/audit', permanent: false },
      { source: '/super-admin', destination: '/admin/organizations', permanent: false },
      { source: '/super-admin/:path*', destination: '/admin/organizations', permanent: false },
    ]
  },
}

export default nextConfig
