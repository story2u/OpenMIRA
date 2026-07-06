import { AlertTriangle } from "lucide-react";
import { Button } from "@/components/ui/button";

export function ErrorState({ message, onRetry }: { message?: string; onRetry?: () => void }) {
  return (
    <div className="flex flex-col items-center justify-center gap-2 py-14 text-center">
      <div className="flex h-9 w-9 items-center justify-center rounded-md bg-destructive/10 text-destructive">
        <AlertTriangle className="h-4 w-4" />
      </div>
      <p className="text-sm font-medium">Couldn't load this data</p>
      <p className="max-w-sm text-xs text-muted-foreground">
        {message ?? "The request failed. Check your connection and try again."}
      </p>
      {onRetry && (
        <Button size="sm" variant="outline" onClick={onRetry}>
          Retry
        </Button>
      )}
    </div>
  );
}
