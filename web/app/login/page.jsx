import { AppShell } from "../../components/AppShell.jsx";
import { LoginPageClient } from "../../components/LoginPageClient.jsx";

export default function LoginPage() {
  return (
    <AppShell active="cs">
      <LoginPageClient mode="passwordless" />
    </AppShell>
  );
}
