import packageJson from "../../package.json";

const currentYear = new Date().getFullYear();

export const APP_CONFIG = {
  name: "AdversaryOps",
  version: packageJson.version,
  copyright: `© ${currentYear} Barrier Networks.`,
  meta: {
    title: "AdversaryOps — Autonomous Penetration Testing Console",
    description: "AdversaryOps by Barrier Networks — console for the LLM-driven autonomous penetration testing platform",
  },
};
