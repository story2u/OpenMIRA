import { Inbox } from "lucide-react";
import { Card, CardContent } from "../ui/card";

export function EmptyState({ title, description }: { title: string; description: string }) {
  return (
    <Card className="border-dashed">
      <CardContent className="grid min-h-44 place-items-center p-8 text-center">
        <div className="grid max-w-sm gap-3">
          <div className="mx-auto grid h-11 w-11 place-items-center rounded-full bg-muted text-muted-foreground">
            <Inbox className="h-5 w-5" aria-hidden="true" />
          </div>
          <div className="grid gap-1">
            <p className="text-sm font-medium text-foreground">{title}</p>
            <p className="text-sm leading-6 text-muted-foreground">{description}</p>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
