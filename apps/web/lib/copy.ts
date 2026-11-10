export type Language = "en" | "hi";

const copy = {
  en: {
    dashboard: "Dashboard",
    nearby: "Nearby",
    reports: "My Reports",
    work: "Work",
    notifications: "Notifications",
    profile: "Profile",
    operations: "Operations",
    reportProblem: "Report a problem",
    signIn: "Sign in",
    createAccount: "Create account",
    signOut: "Sign out",
  },
  hi: {
    dashboard: "डैशबोर्ड",
    nearby: "आस-पास",
    reports: "मेरी रिपोर्ट",
    work: "कार्य",
    notifications: "सूचनाएँ",
    profile: "प्रोफ़ाइल",
    operations: "संचालन",
    reportProblem: "समस्या दर्ज करें",
    signIn: "साइन इन",
    createAccount: "खाता बनाएँ",
    signOut: "साइन आउट",
  },
} as const;

export function text(language: string) {
  return language === "hi" ? copy.hi : copy.en;
}
