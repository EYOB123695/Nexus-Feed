'use client' ; 
import React,{useState} from 'react' ; 
import {QueryClient,QueryClientProvider} from '@tanstack/react-query' ; 

export default function QueryProvider({children}: {children: React.ReactNode}) { 

     // Use useState to ensure the QueryClient is only initialized once per client session

     const [queryClient] = useState (
        () => new QueryClient({
            defaultOptions : { 
                queries : { 
                    staleTime : 1000 * 5 , // 5 seconds 
                    retry : 2 , //Retry failed queries twice before displaying error
                    refetchOnWindowFocus: false,
                },
            },
        })
     );
      return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
    }

