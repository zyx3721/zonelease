import { Loader2 } from 'lucide-react';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Button } from '@/components/ui/button';

interface DhcpConfirmDialogProps {
  open: boolean;
  title: string;
  description: string;
  confirmText: string;
  tone?: 'default' | 'warning' | 'destructive';
  destructive?: boolean;
  loading?: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
}

export function DhcpConfirmDialog({
  open,
  title,
  description,
  confirmText,
  tone,
  destructive = false,
  loading = false,
  onOpenChange,
  onConfirm,
}: DhcpConfirmDialogProps) {
  const buttonTone = tone ?? (destructive ? 'destructive' : 'default');
  const warningClass =
    'border-amber-500/35 bg-amber-500/15 text-amber-700 shadow-sm hover:bg-amber-500/22 dark:text-amber-200';
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          <DialogDescription>{description}</DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" disabled={loading} onClick={() => onOpenChange(false)}>
            取消
          </Button>
          <Button
            variant={buttonTone === 'destructive' ? 'destructive' : 'default'}
            className={buttonTone === 'warning' ? warningClass : undefined}
            disabled={loading}
            onClick={onConfirm}
          >
            {loading ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
            {loading ? '处理中' : confirmText}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
