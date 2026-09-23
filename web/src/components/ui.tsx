import * as React from "react";
import { cn } from "@/lib/utils";
import { ChevronDown } from "lucide-react";

export function Button({ className, variant = "default", size = "default", ...props }: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "default" | "destructive" | "outline" | "ghost" | "secondary"; size?: "default" | "sm" | "icon" }) {
  return (
    <button
      className={cn(
        "inline-flex items-center justify-center gap-1 whitespace-nowrap rounded-md text-sm font-medium transition-colors outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-50",
        variant === "default" && "bg-primary text-primary-foreground shadow hover:bg-primary/85",
        variant === "destructive" && "bg-destructive text-destructive-foreground shadow-sm hover:bg-destructive/85",
        variant === "outline" && "border border-border bg-transparent hover:bg-accent hover:text-accent-foreground",
        variant === "ghost" && "hover:bg-accent hover:text-accent-foreground",
        variant === "secondary" && "bg-muted hover:bg-muted/70",
        size === "default" && "h-9 px-4 py-2",
        size === "sm" && "h-8 px-3 text-xs",
        size === "icon" && "h-8 w-8",
        className
      )}
      {...props}
    />
  );
}

export function Input({ className, ...props }: React.InputHTMLAttributes<HTMLInputElement>) {
  return (
    <input
      className={cn(
        "flex h-9 w-full rounded-md border border-border bg-input/60 px-3 py-1 text-sm outline-none transition-colors placeholder:text-muted-foreground hover:border-muted-foreground/40 focus:border-primary focus:ring-1 focus:ring-ring",
        className
      )}
      {...props}
    />
  );
}

// 下拉框：自带深色适配（bg-input + option 配色见 index.css）与 chevron 图标
export function Select({ className, children, ...props }: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return (
    <div className={cn("relative inline-flex items-center", className)}>
      <select
        className="h-8 cursor-pointer appearance-none rounded-md border border-border bg-input/60 pl-2.5 pr-7 text-sm text-foreground outline-none transition-colors hover:border-muted-foreground/40 focus:border-primary focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
        {...props}
      >
        {children}
      </select>
      <ChevronDown size={13} className="pointer-events-none absolute right-2 text-muted-foreground" />
    </div>
  );
}

export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("rounded-lg border border-border bg-card text-card-foreground shadow-sm", className)} {...props} />;
}

export function Badge({ className, variant = "default", ...props }: React.HTMLAttributes<HTMLSpanElement> & { variant?: "default" | "success" | "destructive" | "outline" }) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium",
        variant === "default" && "bg-muted text-foreground",
        variant === "success" && "bg-green-500/15 text-green-600 dark:text-green-400",
        variant === "destructive" && "bg-red-500/15 text-red-600 dark:text-red-400",
        variant === "outline" && "border border-border",
        className
      )}
      {...props}
    />
  );
}
