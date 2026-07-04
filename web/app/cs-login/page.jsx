import { AppShell } from "../../components/AppShell.jsx";
import { LoginPageClient } from "../../components/LoginPageClient.jsx";

export default function CSLoginPage() {
  return (
    <AppShell active="cs">
      <LoginPageClient mode="cs" />
    </AppShell>
  );
}
