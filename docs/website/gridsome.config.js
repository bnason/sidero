// This is where project configuration and plugin options are located.
// Learn more: https://gridsome.org/docs/config

// Changes here require a server restart.
// To restart press CTRL + C in terminal and run `gridsome develop`

module.exports = {
  siteName: "Sidero",
  icon: {
    favicon: "./src/assets/favicon.png",
    touchicon: "./src/assets/favicon.png",
  },
  siteUrl: process.env.SITE_URL ? process.env.SITE_URL : "https://sidero.dev",
  settings: {
    title: "Bare metal Kubernetes",
    description:
      "A bare metal provisioning system for managing Kubernetes clusters",
    web: process.env.URL_WEB || false,
    twitter: "https://twitter.com/talossystems",
    github: "https://github.com/talos-systems/sidero",
    nav: {
      links: [
        { path: "/docs/v0.2/", title: "Docs" },
        { path: "/releases/", title: "Releases" },
      ],
    },
    dropdownOptions: [
      {
        version: "v0.1",
        url: "/docs/v0.1/",
        latest: false,
        prerelease: false,
      },
      {
        version: "v0.2",
        url: "/docs/v0.2/",
        latest: true,
        prerelease: false,
      },
    ],
  },

  // Allow '.' in slugs (e.g. /docs/v0.1).
  permalinks: {
    slugify: {
      use: "slugify",
      options: { lower: true },
    },
  },

  plugins: [
    {
      use: "gridsome-source-docs",
      options: {
        baseDir: "./content/docs",
        path: "**/*.md",
        typeName: "MarkdownPage",
        pathPrefix: "/docs",
        sidebarOrder: {
          "v0.1": ["Getting Started", "Configuration", "Guides"],
          "v0.2": ["Getting Started", "Configuration", "Guides"],
        },
        remark: {
          externalLinksTarget: "_blank",
          externalLinksRel: ["noopener", "noreferrer"],
          plugins: [
            "gridsome-plugin-remark-mermaid",
            "@gridsome/remark-prismjs",
          ],
        },
      },
    },

    {
      use: "@gridsome/source-graphql",
      options: {
        url: "https://api.github.com/graphql",
        fieldName: "github",
        typeName: "github",
        headers: {
          Authorization: `Bearer ${process.env["GITHUB_TOKEN"]}`,
        },
      },
    },

    {
      use: "gridsome-plugin-tailwindcss",
      options: {
        tailwindConfig: "./tailwind.config.js",
        purgeConfig: {
          // Prevent purging of prism classes.
          whitelistPatternsChildren: [/token$/],
        },
      },
    },

    {
      use: "@gridsome/plugin-google-analytics",
      options: {
        id: process.env.GA_ID ? process.env.GA_ID : "XX-999999999-9",
      },
    },

    {
      use: "@gridsome/plugin-sitemap",
      options: {},
    },
  ],
};
