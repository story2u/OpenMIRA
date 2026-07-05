import { AlertCircle } from "lucide-react";
import { Alert, AlertDescription, AlertTitle } from "../ui/alert";

export function ErrorCard({ message }: { message: string }) {
  return (
    <Alert className="border-[#f2b8b5] bg-[#fff4f2] text-destructive">
      <div className="flex gap-3">
        <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
        <div>
          <AlertTitle>请求失败</AlertTitle>
          <AlertDescription className="text-destructive/80">{message || "请稍后重试"}</AlertDescription>
        </div>
      </div>
    </Alert>
  );
}
