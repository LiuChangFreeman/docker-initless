# RabbitContainer
Boot containers using `checkpoint&restore`    
Eliminate docker container's boot time to less than **1 second**  

## Requirements  

1. docker=17.03
2. criu(mod)    https://github.com/LiuChangFreeman/criu
     
3. runc(mod)  https://github.com/LiuChangFreeman/runc
4. btrfs disk partion for COW storage  

## How does it work?

RabbitContainer works with the **criu** project, which is known as a tool which can recover a group of froozen processes on another machine.   
RabbitContainer uses the `post-copy restore` to make a container runable before the pages are filled into memory which is a background task.  
Time spent on restoring memory pages at *GB* level will be reduced significantly.  

## Example  
```bash
// todo
```