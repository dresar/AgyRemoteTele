import React, { useEffect } from 'react';
import { X } from 'lucide-react';

interface BottomSheetProps {
  isOpen: boolean;
  onClose: () => void;
  title?: string;
  children: React.ReactNode;
}

export const BottomSheet: React.FC<BottomSheetProps> = ({
  isOpen,
  onClose,
  title,
  children
}) => {
  useEffect(() => {
    if (isOpen) {
      document.body.style.overflow = 'hidden';
    } else {
      document.body.style.overflow = '';
    }
    return () => {
      document.body.style.overflow = '';
    };
  }, [isOpen]);

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col justify-end">
      <div
        onClick={onClose}
        className="fixed inset-0 bg-slate-900/40 backdrop-blur-xs transition-opacity duration-150 motion-reduced:transition-none"
      />
      <div className="relative z-10 w-full max-w-md mx-auto bg-white border-t border-x border-slate-200 rounded-t-[4px] shadow-2xl max-h-[85vh] flex flex-col transition-all duration-200 motion-reduced:transition-opacity motion-reduced:duration-150 overflow-hidden">
        <div className="w-10 h-1 bg-slate-300 rounded-full mx-auto mt-2.5 mb-2 shrink-0 cursor-grab" />
        <div className="flex items-center justify-between px-4 py-2 border-b border-slate-100 shrink-0">
          <h2 className="text-sm font-semibold text-slate-900 tracking-tight">
            {title || 'Detail'}
          </h2>
          <button
            onClick={onClose}
            className="p-1 rounded-[4px] text-slate-400 hover:text-slate-700 hover:bg-slate-100 transition-colors"
          >
            <X className="w-4 h-4" />
          </button>
        </div>
        <div className="p-4 overflow-y-auto overscroll-contain safe-bottom">
          {children}
        </div>
      </div>
    </div>
  );
};
