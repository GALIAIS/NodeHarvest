import Link from "next/link";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export function AuthRequired({
  reason = "anonymous",
  title,
  description,
}: {
  reason?: "anonymous" | "forbidden";
  title?: string;
  description?: string;
}) {
  const forbidden = reason === "forbidden";

  return (
    <Card>
      <CardHeader>
        <CardTitle>{title ?? (forbidden ? "当前角色无权查看" : "需要登录")}</CardTitle>
        <CardDescription>
          {description ??
            (forbidden
              ? "该操作需要更高权限，请联系管理员调整角色。"
              : "该视图属于管理面，登录后即可查看。未登录时仅可查看仪表盘。")}
        </CardDescription>
      </CardHeader>
      {!forbidden && (
        <CardContent>
          <Button asChild>
            <Link href="/login">前往登录</Link>
          </Button>
        </CardContent>
      )}
    </Card>
  );
}
